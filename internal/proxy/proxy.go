// Package proxy implements the HTTP GET caching proxy and CONNECT tunnel.
package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/SagnikSamanta445/webserver/internal/cache"
	"github.com/SagnikSamanta445/webserver/internal/stats"
)

const (
	readBufSize    = 4096
	maxHeaderBytes = 64 << 10
	dialTimeout    = 10 * time.Second
	headerTimeout  = 30 * time.Second
)

var (
	resp400         = []byte("HTTP/1.1 400 Bad Request\r\n\r\n")
	resp500         = []byte("HTTP/1.1 500 Internal Server Error\r\n\r\n")
	respEstablished = []byte("HTTP/1.1 200 Connection Established\r\n\r\n")
)

// Server serves proxy connections, one goroutine per client.
type Server struct {
	Cache *cache.LRU
	Stats *stats.Stats

	// Dial opens origin connections; defaults to net.DialTimeout.
	Dial func(network, addr string) (net.Conn, error)
	// Logger receives connection-level errors; nil discards them.
	Logger *log.Logger
}

// New returns a Server using c and s.
func New(c *cache.LRU, s *stats.Stats) *Server {
	return &Server{Cache: c, Stats: s}
}

// Serve accepts connections on ln until it is closed, handling each client
// concurrently.
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				s.logf("accept failed: %v", err)
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return err
		}
		go s.HandleConn(conn)
	}
}

// HandleConn processes a single client request and closes conn.
func (s *Server) HandleConn(conn net.Conn) {
	defer conn.Close()

	lr := &headerLimitReader{r: conn, n: maxHeaderBytes}
	br := bufio.NewReaderSize(lr, readBufSize)

	_ = conn.SetReadDeadline(time.Now().Add(headerTimeout))
	req, err := ReadRequest(br)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		if errors.Is(err, ErrBadRequest) {
			s.Stats.RecordError()
			conn.Write(resp400)
		}
		return
	}
	lr.disable()

	switch req.Method {
	case "CONNECT":
		s.Stats.RecordConnect()
		s.handleConnect(conn, br, req)
	case "GET":
		s.Stats.RecordGet()
		s.handleGet(conn, req)
	}
}

func (s *Server) handleGet(client net.Conn, req *Request) {
	key := req.CacheKey()
	if data, ok := s.Cache.Get(key); ok {
		client.Write(data)
		s.Stats.RecordBytesCache(len(data))
		return
	}

	origin, err := s.dial(req.Addr())
	if err != nil {
		s.logf("GET %s: %v", key, err)
		s.Stats.RecordError()
		client.Write(resp500)
		return
	}
	defer origin.Close()

	if _, err := origin.Write(buildOriginRequest(req)); err != nil {
		s.logf("GET %s: write origin: %v", key, err)
		return
	}

	// Stream the response to the client while buffering it for the cache.
	// Like the C build, keep draining the origin if the client goes away so
	// the full response can still be cached.
	var body bytes.Buffer
	cacheable := true
	clientOK := true
	buf := make([]byte, readBufSize)
	total := 0
	for {
		n, rerr := origin.Read(buf)
		if n > 0 {
			total += n
			if clientOK {
				if _, werr := client.Write(buf[:n]); werr != nil {
					clientOK = false
				}
			}
			if cacheable {
				if int64(body.Len()+n) > cache.MaxCacheSize {
					cacheable = false
					body = bytes.Buffer{}
				} else {
					body.Write(buf[:n])
				}
			}
		}
		if rerr != nil {
			break
		}
	}

	s.Stats.RecordBytesOrigin(total)
	if cacheable {
		s.Cache.Put(key, body.Bytes())
	}
}

func buildOriginRequest(req *Request) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "GET %s %s\r\n", req.Path, req.Version)
	h := req.Header.Clone()
	h.Del("Host")
	h.Set("Connection", "close")
	fmt.Fprintf(&b, "Host: %s\r\n", hostHeader(req))
	h.Write(&b)
	b.WriteString("\r\n")
	return b.Bytes()
}

func hostHeader(req *Request) string {
	if req.explicitPort {
		return req.Addr()
	}
	if ip := net.ParseIP(req.Host); ip != nil && ip.To4() == nil {
		return "[" + req.Host + "]"
	}
	return req.Host
}

func (s *Server) handleConnect(client net.Conn, br *bufio.Reader, req *Request) {
	remote, err := s.dial(req.Addr())
	if err != nil {
		s.logf("CONNECT %s: %v", req.Addr(), err)
		client.Write(resp500)
		return
	}
	defer remote.Close()

	if _, err := client.Write(respEstablished); err != nil {
		return
	}
	tunnel(client, br, remote)
}

// tunnel relays bytes in both directions until either side finishes, then
// closes both. Bytes the client sent after the CONNECT head (already
// buffered in br) are forwarded first.
func tunnel(client net.Conn, br *bufio.Reader, remote net.Conn) {
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			client.Close()
			remote.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(remote, br)
		closeBoth()
	}()
	go func() {
		defer wg.Done()
		io.Copy(client, remote)
		closeBoth()
	}()
	wg.Wait()
}

func (s *Server) dial(addr string) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial("tcp", addr)
	}
	return net.DialTimeout("tcp", addr, dialTimeout)
}

func (s *Server) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// headerLimitReader caps how many bytes may be read while parsing the
// request head; disable lifts the cap for tunnelled traffic.
type headerLimitReader struct {
	r        io.Reader
	n        int64
	disabled bool
}

var errHeaderTooLarge = errors.New("request header too large")

func (l *headerLimitReader) Read(p []byte) (int, error) {
	if l.disabled {
		return l.r.Read(p)
	}
	if l.n <= 0 {
		return 0, errHeaderTooLarge
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func (l *headerLimitReader) disable() { l.disabled = true }
