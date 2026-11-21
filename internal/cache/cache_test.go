package cache

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestGetMissAndHit(t *testing.T) {
	c := New(100)
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected miss on empty cache")
	}
	c.Put("a", []byte("hello"))
	got, ok := c.Get("a")
	if !ok || string(got) != "hello" {
		t.Fatalf("Get(a) = %q, %v", got, ok)
	}
	s := c.Snapshot()
	if s.Lookups != 2 || s.Hits != 1 || s.HitRatio() != 0.5 {
		t.Fatalf("unexpected lookup stats: %+v", s)
	}
}

func TestPutCopiesData(t *testing.T) {
	c := New(100)
	data := []byte("abc")
	c.Put("k", data)
	data[0] = 'x'
	got, _ := c.Get("k")
	if string(got) != "abc" {
		t.Fatalf("cache aliased caller buffer: %q", got)
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New(30)
	var evicted []string
	c.OnEvict = func(key string, _ int) { evicted = append(evicted, key) }

	c.Put("a", bytes.Repeat([]byte("a"), 10))
	c.Put("b", bytes.Repeat([]byte("b"), 10))
	c.Put("c", bytes.Repeat([]byte("c"), 10))

	// Touch "a" so "b" becomes the LRU entry.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected hit for a")
	}
	c.Put("d", bytes.Repeat([]byte("d"), 10))

	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("%s should still be cached", k)
		}
	}
	s := c.Snapshot()
	if s.Evictions != 1 || s.Entries != 3 || s.Size != 30 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if len(evicted) != 1 || evicted[0] != "b" {
		t.Fatalf("OnEvict calls = %v, want [b]", evicted)
	}
}

func TestEvictsMultipleToFitLargeEntry(t *testing.T) {
	c := New(30)
	c.Put("a", make([]byte, 10))
	c.Put("b", make([]byte, 10))
	c.Put("c", make([]byte, 10))
	c.Put("big", make([]byte, 25))

	s := c.Snapshot()
	if s.Entries != 1 || s.Size != 25 || s.Evictions != 3 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
}

func TestUpdateExistingKeyResizesAndEvicts(t *testing.T) {
	c := New(30)
	c.Put("a", make([]byte, 10))
	c.Put("b", make([]byte, 10))
	c.Put("b", make([]byte, 25)) // grows b; a must be evicted

	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have been evicted")
	}
	s := c.Snapshot()
	if s.Entries != 1 || s.Size != 25 || s.Evictions != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
}

func TestRejectsEmptyAndOversizedValues(t *testing.T) {
	c := New(10)
	c.Put("empty", nil)
	c.Put("huge", make([]byte, 11))
	if s := c.Snapshot(); s.Entries != 0 || s.Size != 0 {
		t.Fatalf("expected empty cache, got %+v", s)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(1 << 10)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				k := fmt.Sprintf("k%d", (g*i)%64)
				c.Put(k, make([]byte, 1+i%32))
				c.Get(k)
				c.Snapshot()
			}
		}(g)
	}
	wg.Wait()
	if s := c.Snapshot(); s.Size > s.MaxSize {
		t.Fatalf("cache exceeded capacity: %+v", s)
	}
}
