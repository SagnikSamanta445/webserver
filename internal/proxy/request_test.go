package proxy

import (
	"bufio"
	"errors"
	"strings"
	"testing"
)

func parse(t *testing.T, raw string) (*Request, error) {
	t.Helper()
	return ReadRequest(bufio.NewReader(strings.NewReader(raw)))
}

func TestRequestValidation(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"get http/1.1", "GET http://example.com/index.html HTTP/1.1\r\nHost: example.com\r\n\r\n", true},
		{"get http/1.0", "GET http://example.com/ HTTP/1.0\r\n\r\n", true},
		{"get no path", "GET http://example.com HTTP/1.1\r\n\r\n", true},
		{"connect", "CONNECT example.com:443 HTTP/1.1\r\n\r\n", true},
		{"connect no port", "CONNECT example.com HTTP/1.1\r\n\r\n", true},
		{"post rejected", "POST http://example.com/ HTTP/1.1\r\n\r\n", false},
		{"head rejected", "HEAD http://example.com/ HTTP/1.1\r\n\r\n", false},
		{"origin-form rejected", "GET /index.html HTTP/1.1\r\nHost: example.com\r\n\r\n", false},
		{"http/2 rejected", "GET http://example.com/ HTTP/2.0\r\n\r\n", false},
		{"http/0.9 rejected", "GET http://example.com/ HTTP/0.9\r\n\r\n", false},
		{"missing version", "GET http://example.com/\r\n\r\n", false},
		{"garbage", "hello\r\n\r\n", false},
		{"double slash path", "GET http://example.com//x HTTP/1.1\r\n\r\n", false},
		{"connect missing host", "CONNECT :443 HTTP/1.1\r\n\r\n", false},
		{"bad header line", "GET http://example.com/ HTTP/1.1\r\nnocolon\r\n\r\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(t, tc.raw)
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrBadRequest) {
				t.Fatalf("expected ErrBadRequest, got %v", err)
			}
		})
	}
}

func TestEmptyConnectionIsNotBadRequest(t *testing.T) {
	_, err := parse(t, "")
	if err == nil || errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected plain EOF error, got %v", err)
	}
}

func TestConnectDefaultsToPort443(t *testing.T) {
	cases := map[string]string{
		"CONNECT example.com HTTP/1.1\r\n\r\n":      "example.com:443",
		"CONNECT example.com:443 HTTP/1.1\r\n\r\n":  "example.com:443",
		"CONNECT example.com:8443 HTTP/1.1\r\n\r\n": "example.com:8443",
		"CONNECT [::1] HTTP/1.1\r\n\r\n":            "[::1]:443",
		"CONNECT [::1]:9000 HTTP/1.1\r\n\r\n":       "[::1]:9000",
	}
	for raw, want := range cases {
		req, err := parse(t, raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if req.Method != "CONNECT" || req.Addr() != want {
			t.Errorf("%q: Addr() = %q, want %q", raw, req.Addr(), want)
		}
	}
}

func TestGetDefaultsToPort80AndCacheKey(t *testing.T) {
	cases := []struct {
		raw, addr, path, key, version string
	}{
		{"GET http://example.com HTTP/1.1\r\n\r\n", "example.com:80", "/", "example.com/", "HTTP/1.1"},
		{"GET http://example.com/a/b?q=1 HTTP/1.0\r\n\r\n", "example.com:80", "/a/b?q=1", "example.com/a/b?q=1", "HTTP/1.0"},
		{"GET http://example.com:80/x HTTP/1.1\r\n\r\n", "example.com:80", "/x", "example.com/x", "HTTP/1.1"},
		{"GET http://example.com:8080/x HTTP/1.1\r\n\r\n", "example.com:8080", "/x", "example.com:8080/x", "HTTP/1.1"},
	}
	for _, tc := range cases {
		req, err := parse(t, tc.raw)
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if req.Addr() != tc.addr || req.Path != tc.path || req.CacheKey() != tc.key || req.Version != tc.version {
			t.Errorf("%q: got addr=%q path=%q key=%q version=%q", tc.raw, req.Addr(), req.Path, req.CacheKey(), req.Version)
		}
	}
}

func TestBuildOriginRequest(t *testing.T) {
	req, err := parse(t, "GET http://example.com/p HTTP/1.1\r\nUser-Agent: test\r\nConnection: keep-alive\r\n\r\n")
	if err != nil {
		t.Fatal(err)
	}
	out := string(buildOriginRequest(req))
	for _, want := range []string{
		"GET /p HTTP/1.1\r\n",
		"Host: example.com\r\n",
		"Connection: close\r\n",
		"User-Agent: test\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("origin request missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "keep-alive") || !strings.HasSuffix(out, "\r\n\r\n") {
		t.Errorf("malformed origin request:\n%q", out)
	}
}
