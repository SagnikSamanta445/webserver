package stats

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SagnikSamanta445/webserver/internal/cache"
)

func TestAccounting(t *testing.T) {
	s := New()
	s.RecordGet()
	s.RecordGet()
	s.RecordConnect()
	s.RecordError()
	s.RecordBytesOrigin(1000)
	s.RecordBytesCache(500)
	s.RecordBytesCache(24)

	got := s.Snapshot()
	want := Snapshot{Start: got.Start, Total: 4, Gets: 2, Connects: 1, Errors: 1, BytesOrigin: 1000, BytesCache: 524}
	if got != want {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestConcurrentAccounting(t *testing.T) {
	s := New()
	const workers, iters = 16, 1000
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				s.RecordGet()
				s.RecordConnect()
				s.RecordError()
				s.RecordBytesOrigin(2)
				s.RecordBytesCache(1)
			}
		}()
	}
	wg.Wait()

	n := uint64(workers * iters)
	got := s.Snapshot()
	if got.Total != 3*n || got.Gets != n || got.Connects != n || got.Errors != n ||
		got.BytesOrigin != 2*n || got.BytesCache != n {
		t.Fatalf("unexpected totals: %+v", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{
		0:               "0 B",
		1023:            "1023 B",
		1024:            "1.0 KB",
		1536:            "1.5 KB",
		1 << 20:         "1.0 MB",
		200 << 20:       "200.0 MB",
		5*(1<<20) + 1e5: "5.1 MB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteReport(t *testing.T) {
	start := time.Unix(1000, 0)
	s := Snapshot{Start: start, Total: 20, Gets: 12, Connects: 6, Errors: 2, BytesOrigin: 3072, BytesCache: 1024}
	c := cache.Snapshot{Entries: 3, Size: 2048, MaxSize: cache.MaxCacheSize, Evictions: 1, Lookups: 4, Hits: 1}

	var buf bytes.Buffer
	WriteReport(&buf, s, c, start.Add(10*time.Second))
	out := buf.String()

	for _, want := range []string{
		"[uptime: 10s | 2.00 req/s]",
		"Total: 20      GET: 12      CONNECT: 6",
		"Errors: 2      Error rate: 10.00%",
		"Hit ratio: 25.00%   Evictions: 1",
		"Size: 2.0 KB / 200.0 MB  Entries: 3",
		"From origin: 3.0 KB   From cache: 1.0 KB",
		"Bandwidth saved: 25.00%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

func TestRunReporterStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		RunReporter(ctx, 5*time.Millisecond, func() { calls.Add(1) })
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reporter did not stop after cancel")
	}
	if calls.Load() == 0 {
		t.Fatal("reporter never fired")
	}
}
