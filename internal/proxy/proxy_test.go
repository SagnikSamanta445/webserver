package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SagnikSamanta445/webserver/internal/cache"
	"github.com/SagnikSamanta445/webserver/internal/stats"
)

func startProxy(t *testing.T) (*Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(cache.New(cache.MaxCacheSize), stats.New())
	go srv.Serve(ln)
	t.Cleanup(func() { ln.Close() })
	return srv, ln.Addr().String()
}

// startOrigin serves a fixed HTTP response and counts requests.
func startOrigin(t *testing.T, body string) (addr string, hits *atomic.Int32, lastReq chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	hits = new(atomic.Int32)
	lastReq = make(chan string, 16)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)
				var head strings.Builder
				for {
					line, err := br.ReadString('\n')
					head.WriteString(line)
					if err != nil || line == "\r\n" {
						break
					}
				}
				hits.Add(1)
				select {
				case lastReq <- head.String():
				default:
				}
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
			}(c)
		}
	}()
	return ln.Addr().String(), hits, lastReq
}

func roundTrip(t *testing.T, proxyAddr, raw string) string {
	t.Helper()
	c, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(c, raw); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestGetProxiesAndCaches(t *testing.T) {
	srv, proxyAddr := startProxy(t)
	origin, hits, lastReq := startOrigin(t, "hello world")
	raw := fmt.Sprintf("GET http://%s/greeting HTTP/1.1\r\nHost: %s\r\n\r\n", origin, origin)

	first := roundTrip(t, proxyAddr, raw)
	if !strings.HasSuffix(first, "hello world") || !strings.HasPrefix(first, "HTTP/1.1 200 OK") {
		t.Fatalf("unexpected first response: %q", first)
	}
	head := <-lastReq
	if !strings.HasPrefix(head, "GET /greeting HTTP/1.1\r\n") || !strings.Contains(head, "Connection: close\r\n") {
		t.Fatalf("unexpected origin request: %q", head)
	}

	second := roundTrip(t, proxyAddr, raw)
	if second != first {
		t.Fatalf("cached response differs:\n%q\n%q", second, first)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("origin hit %d times, want 1 (second request should be cached)", n)
	}

	s := srv.Stats.Snapshot()
	if s.Total != 2 || s.Gets != 2 || s.Errors != 0 ||
		s.BytesOrigin != uint64(len(first)) || s.BytesCache != uint64(len(first)) {
		t.Fatalf("unexpected stats: %+v", s)
	}
	if c := srv.Cache.Snapshot(); c.Entries != 1 || c.Hits != 1 || c.Lookups != 2 {
		t.Fatalf("unexpected cache stats: %+v", c)
	}
}

func TestBadRequestReturns400(t *testing.T) {
	srv, proxyAddr := startProxy(t)
	out := roundTrip(t, proxyAddr, "POST http://example.com/ HTTP/1.1\r\n\r\n")
	if out != "HTTP/1.1 400 Bad Request\r\n\r\n" {
		t.Fatalf("got %q", out)
	}
	if s := srv.Stats.Snapshot(); s.Errors != 1 || s.Total != 1 {
		t.Fatalf("unexpected stats: %+v", s)
	}
}

func TestUnreachableOriginReturns500(t *testing.T) {
	srv, proxyAddr := startProxy(t)
	srv.Dial = func(string, string) (net.Conn, error) { return nil, fmt.Errorf("refused") }

	out := roundTrip(t, proxyAddr, "GET http://example.invalid/ HTTP/1.1\r\n\r\n")
	if out != "HTTP/1.1 500 Internal Server Error\r\n\r\n" {
		t.Fatalf("got %q", out)
	}
	// Matches the C build: the GET is counted, then the failure is counted.
	if s := srv.Stats.Snapshot(); s.Gets != 1 || s.Errors != 1 || s.Total != 2 {
		t.Fatalf("unexpected stats: %+v", s)
	}

	out = roundTrip(t, proxyAddr, "CONNECT example.invalid HTTP/1.1\r\n\r\n")
	if out != "HTTP/1.1 500 Internal Server Error\r\n\r\n" {
		t.Fatalf("got %q", out)
	}
}

func TestConnectTunnel(t *testing.T) {
	srv, proxyAddr := startProxy(t)

	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		c, err := echo.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		io.Copy(c, c)
	}()

	var dialed atomic.Value
	srv.Dial = func(network, addr string) (net.Conn, error) {
		dialed.Store(addr)
		return net.Dial(network, echo.Addr().String())
	}

	c, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))

	// Data pipelined right after the CONNECT head must reach the remote.
	io.WriteString(c, "CONNECT secure.example HTTP/1.1\r\n\r\nping")
	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil || status != "HTTP/1.1 200 Connection Established\r\n" {
		t.Fatalf("status = %q, %v", status, err)
	}
	if blank, _ := br.ReadString('\n'); blank != "\r\n" {
		t.Fatalf("expected blank line, got %q", blank)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(br, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
	io.WriteString(c, "pong")
	if _, err := io.ReadFull(br, buf); err != nil || string(buf) != "pong" {
		t.Fatalf("echo = %q, %v", buf, err)
	}

	if got := dialed.Load(); got != "secure.example:443" {
		t.Fatalf("dialed %v, want secure.example:443", got)
	}
	if s := srv.Stats.Snapshot(); s.Connects != 1 || s.Total != 1 {
		t.Fatalf("unexpected stats: %+v", s)
	}
}

func TestConcurrentClients(t *testing.T) {
	_, proxyAddr := startProxy(t)
	origin, _, _ := startOrigin(t, "ok")

	const n = 32
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			raw := fmt.Sprintf("GET http://%s/item/%d HTTP/1.0\r\n\r\n", origin, i%4)
			c, err := net.Dial("tcp", proxyAddr)
			if err != nil {
				errs <- err
				return
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(5 * time.Second))
			io.WriteString(c, raw)
			out, err := io.ReadAll(c)
			if err == nil && !strings.HasSuffix(string(out), "ok") {
				err = fmt.Errorf("bad response %q", out)
			}
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}
