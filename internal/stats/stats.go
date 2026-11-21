// Package stats tracks proxy request and bandwidth counters and renders the
// periodic stats report.
package stats

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/SagnikSamanta445/webserver/internal/cache"
)

// Stats holds proxy counters. All methods are safe for concurrent use.
type Stats struct {
	start time.Time

	total       atomic.Uint64
	gets        atomic.Uint64
	connects    atomic.Uint64
	errors      atomic.Uint64
	bytesOrigin atomic.Uint64
	bytesCache  atomic.Uint64
}

// Snapshot is a point-in-time copy of the counters.
type Snapshot struct {
	Start       time.Time
	Total       uint64
	Gets        uint64
	Connects    uint64
	Errors      uint64
	BytesOrigin uint64
	BytesCache  uint64
}

// New returns zeroed counters with the uptime clock started now.
func New() *Stats {
	return &Stats{start: time.Now()}
}

// RecordGet counts a valid GET request.
func (s *Stats) RecordGet() {
	s.total.Add(1)
	s.gets.Add(1)
}

// RecordConnect counts a CONNECT request.
func (s *Stats) RecordConnect() {
	s.total.Add(1)
	s.connects.Add(1)
}

// RecordError counts a failed request. Like the C implementation, every
// recorded error also increments the total request count.
func (s *Stats) RecordError() {
	s.total.Add(1)
	s.errors.Add(1)
}

// RecordBytesOrigin adds bytes received from origin servers.
func (s *Stats) RecordBytesOrigin(n int) { s.bytesOrigin.Add(uint64(n)) }

// RecordBytesCache adds bytes served from the cache.
func (s *Stats) RecordBytesCache(n int) { s.bytesCache.Add(uint64(n)) }

// Snapshot returns the current counter values.
func (s *Stats) Snapshot() Snapshot {
	return Snapshot{
		Start:       s.start,
		Total:       s.total.Load(),
		Gets:        s.gets.Load(),
		Connects:    s.connects.Load(),
		Errors:      s.errors.Load(),
		BytesOrigin: s.bytesOrigin.Load(),
		BytesCache:  s.bytesCache.Load(),
	}
}

// FormatBytes renders n as B, KB or MB with the same thresholds as the C build.
func FormatBytes(n uint64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// WriteReport writes the stats banner for s and c to w, evaluated at now.
func WriteReport(w io.Writer, s Snapshot, c cache.Snapshot, now time.Time) {
	uptime := int64(now.Sub(s.Start) / time.Second)
	var rps, errRate, bwSaved float64
	if uptime > 0 {
		rps = float64(s.Total) / float64(uptime)
	}
	if s.Total > 0 {
		errRate = 100 * float64(s.Errors) / float64(s.Total)
	}
	if served := s.BytesOrigin + s.BytesCache; served > 0 {
		bwSaved = 100 * float64(s.BytesCache) / float64(served)
	}

	fmt.Fprintf(w, "\n========================================================\n")
	fmt.Fprintf(w, "  PROXY STATS  [uptime: %ds | %.2f req/s]\n", uptime, rps)
	fmt.Fprintf(w, "========================================================\n")
	fmt.Fprintf(w, "  REQUESTS\n")
	fmt.Fprintf(w, "    Total: %-6d  GET: %-6d  CONNECT: %-6d\n", s.Total, s.Gets, s.Connects)
	fmt.Fprintf(w, "    Errors: %-5d  Error rate: %.2f%%\n", s.Errors, errRate)
	fmt.Fprintf(w, "\n  CACHE\n")
	fmt.Fprintf(w, "    Hit ratio: %.2f%%   Evictions: %d\n", c.HitRatio()*100, c.Evictions)
	fmt.Fprintf(w, "    Size: %s / %s  Entries: %d\n",
		FormatBytes(uint64(c.Size)), FormatBytes(uint64(c.MaxSize)), c.Entries)
	fmt.Fprintf(w, "\n  BANDWIDTH\n")
	fmt.Fprintf(w, "    From origin: %s   From cache: %s\n",
		FormatBytes(s.BytesOrigin), FormatBytes(s.BytesCache))
	fmt.Fprintf(w, "    Bandwidth saved: %.2f%%\n", bwSaved)
	fmt.Fprintf(w, "========================================================\n")
}

// RunReporter calls report every interval until ctx is cancelled.
func RunReporter(ctx context.Context, interval time.Duration, report func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			report()
		}
	}
}
