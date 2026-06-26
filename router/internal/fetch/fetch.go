// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/safeurl"
)

type Request struct {
	URL     string
	Mode    string
	APIKey  string
	BaseURL string
	Extra   map[string]any
}

type Result struct {
	URL         string
	Title       string
	Body        []byte
	ContentType string
	StatusCode  int
}

type Fetcher interface {
	Name() string
	Fetch(ctx context.Context, req Request) (Result, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]Fetcher{}
)

func Register(p Fetcher) {
	if p == nil || p.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[p.Name()] = p
}

func Get(name string) Fetcher {
	regMu.RLock()
	defer regMu.RUnlock()
	return registry[name]
}

func List() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

var fetchHTTPClient = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if _, err := safeurl.Validate(req.Context(), req.URL.String()); err != nil {
			return fmt.Errorf("redirect blocked: %w", err)
		}
		return nil
	},
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

var allowPrivateDial = false

func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivateDial && !safeurl.IsPublic(ip) {
			return nil, fmt.Errorf("%w: %s", safeurl.ErrBlocked, ip)
		}
		return d.DialContext(ctx, network, addr)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, a := range ips {
		if !allowPrivateDial && !safeurl.IsPublic(a.IP) {
			return nil, fmt.Errorf("%w: %s -> %s", safeurl.ErrBlocked, host, a.IP)
		}
	}

	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

func doHTTPRequest(r *http.Request) ([]byte, *http.Response, error) {
	resp, err := fetchHTTPClient.Do(r)
	if err != nil {
		return nil, nil, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	return body, resp, nil
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
