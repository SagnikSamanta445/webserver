package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const (
	defaultHTTPPort    = "80"
	defaultConnectPort = "443"
)

// ErrBadRequest wraps every request validation failure.
var ErrBadRequest = errors.New("bad request")

// Request is a validated proxy request.
type Request struct {
	Method  string // "GET" or "CONNECT"
	Host    string // hostname without port (IPv6 literals unbracketed)
	Port    string // explicit port, or 80 (GET) / 443 (CONNECT)
	Path    string // origin-form request target for GET, always starting with "/"
	Version string // "HTTP/1.0" or "HTTP/1.1"
	Header  http.Header

	explicitPort bool
}

// Addr is the host:port to dial.
func (r *Request) Addr() string { return net.JoinHostPort(r.Host, r.Port) }

// CacheKey is host+path. An explicit non-default port is included in the
// host so different origins on one host do not share entries.
func (r *Request) CacheKey() string {
	host := r.Host
	if r.explicitPort && r.Port != defaultHTTPPort {
		host = r.Addr()
	}
	return host + r.Path
}

func badRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrBadRequest, fmt.Sprintf(format, args...))
}

// ReadRequest reads one request head from br and validates it. I/O errors
// (including EOF) are returned unwrapped; validation failures wrap
// ErrBadRequest.
func ReadRequest(br *bufio.Reader) (*Request, error) {
	hr, err := http.ReadRequest(br)
	if err != nil {
		var pe *net.OpError
		if errors.As(err, &pe) || isEOF(err) {
			return nil, err
		}
		return nil, badRequest("%v", err)
	}
	return validate(hr)
}

func validate(hr *http.Request) (*Request, error) {
	if hr.Proto != "HTTP/1.0" && hr.Proto != "HTTP/1.1" {
		return nil, badRequest("unsupported version %q", hr.Proto)
	}

	switch hr.Method {
	case http.MethodConnect:
		host, port := hr.URL.Hostname(), hr.URL.Port()
		if host == "" {
			return nil, badRequest("CONNECT missing host")
		}
		explicit := port != ""
		if !explicit {
			port = defaultConnectPort
		}
		return &Request{
			Method: hr.Method, Host: host, Port: port,
			Version: hr.Proto, Header: hr.Header, explicitPort: explicit,
		}, nil

	case http.MethodGet:
		if !hr.URL.IsAbs() || hr.URL.Host == "" {
			return nil, badRequest("GET requires an absolute URI, got %q", hr.RequestURI)
		}
		host, port := hr.URL.Hostname(), hr.URL.Port()
		if host == "" {
			return nil, badRequest("GET missing host")
		}
		explicit := port != ""
		if !explicit {
			port = defaultHTTPPort
		}
		path := hr.URL.RequestURI()
		if strings.HasPrefix(path, "//") {
			return nil, badRequest("path cannot begin with two slashes")
		}
		return &Request{
			Method: hr.Method, Host: host, Port: port, Path: path,
			Version: hr.Proto, Header: hr.Header, explicitPort: explicit,
		}, nil

	default:
		return nil, badRequest("unsupported method %q", hr.Method)
	}
}
