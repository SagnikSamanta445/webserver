// Package cache implements a thread-safe, size-bounded LRU cache keyed by
// request URL (host+path).
package cache

import (
	"container/list"
	"sync"
)

// MaxCacheSize is the default capacity of the proxy cache (200 MB).
const MaxCacheSize int64 = 200 << 20

type entry struct {
	key  string
	data []byte
}

// LRU is a byte-size bounded least-recently-used cache. The front of the
// list holds the most recently used entry.
type LRU struct {
	mu        sync.RWMutex
	maxSize   int64
	size      int64
	ll        *list.List
	items     map[string]*list.Element
	evictions uint64
	lookups   uint64
	hits      uint64

	// OnEvict, if set, is called (with the lock held) for each evicted entry.
	OnEvict func(key string, size int)
}

// Snapshot is a point-in-time view of cache metrics.
type Snapshot struct {
	Entries   int
	Size      int64
	MaxSize   int64
	Evictions uint64
	Lookups   uint64
	Hits      uint64
}

// HitRatio returns hits/lookups, or 0 when there have been no lookups.
func (s Snapshot) HitRatio() float64 {
	if s.Lookups == 0 {
		return 0
	}
	return float64(s.Hits) / float64(s.Lookups)
}

// New returns an empty cache bounded to maxSize bytes.
func New(maxSize int64) *LRU {
	return &LRU{
		maxSize: maxSize,
		ll:      list.New(),
		items:   make(map[string]*list.Element),
	}
}

// Get returns the cached data for key and marks it most recently used.
// The returned slice is shared with the cache and must not be modified.
func (c *LRU) Get(key string) ([]byte, bool) {
	// A write lock is required: a hit reorders the LRU list.
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lookups++
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.hits++
	c.ll.MoveToFront(el)
	return el.Value.(*entry).data, true
}

// Put stores a copy of data under key, evicting least recently used entries
// until the cache fits. Empty values and values larger than the cache
// capacity are ignored.
func (c *LRU) Put(key string, data []byte) {
	n := int64(len(data))
	if n <= 0 || n > c.maxSize {
		return
	}
	buf := make([]byte, n)
	copy(buf, data)

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		c.size += n - int64(len(e.data))
		e.data = buf
		c.ll.MoveToFront(el)
		for c.size > c.maxSize {
			c.evictOldest()
		}
		return
	}

	for c.size+n > c.maxSize {
		c.evictOldest()
	}
	c.items[key] = c.ll.PushFront(&entry{key: key, data: buf})
	c.size += n
}

func (c *LRU) evictOldest() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	e := el.Value.(*entry)
	c.ll.Remove(el)
	delete(c.items, e.key)
	c.size -= int64(len(e.data))
	c.evictions++
	if c.OnEvict != nil {
		c.OnEvict(e.key, len(e.data))
	}
}

// Snapshot returns the current cache metrics.
func (c *LRU) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Snapshot{
		Entries:   len(c.items),
		Size:      c.size,
		MaxSize:   c.maxSize,
		Evictions: c.evictions,
		Lookups:   c.lookups,
		Hits:      c.hits,
	}
}
