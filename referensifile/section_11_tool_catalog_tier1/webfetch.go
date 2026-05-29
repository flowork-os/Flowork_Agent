package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// ErrSSRFBlocked signals a webfetch attempt to a disallowed internal IP.
// Caller receives this wrapped inside the tool error.
var ErrSSRFBlocked = errors.New("SSRF blocked: URL resolves to private/loopback/metadata IP")

// ssrfBlockedCIDRs lists address ranges that webfetch refuses to dial.
// Gemini audit BUG_2.MD Bug 1: without this, agent-visible URLs could
// exfiltrate cloud IAM credentials (169.254.169.254), hit docker daemon
// (127.0.0.1:2375), or pivot into LAN services.
var ssrfBlockedCIDRs = []string{
	"127.0.0.0/8",        // IPv4 loopback
	"10.0.0.0/8",         // RFC 1918 private
	"172.16.0.0/12",      // RFC 1918 private
	"192.168.0.0/16",     // RFC 1918 private
	"169.254.0.0/16",     // Link-local + AWS/GCP/Azure metadata
	"100.64.0.0/10",      // CGNAT / Tailscale
	"0.0.0.0/8",          // "This network"
	"224.0.0.0/4",        // Audit GAP #7 — IPv4 multicast
	"240.0.0.0/4",        // Reserved / future use
	"255.255.255.255/32", // Broadcast
	"::1/128",            // IPv6 loopback
	"::/128",             // IPv6 unspecified
	"fc00::/7",           // IPv6 unique local
	"fe80::/10",          // IPv6 link-local
	"ff00::/8",           // IPv6 multicast
}

var blockedNets []*net.IPNet

func init() {
	for _, c := range ssrfBlockedCIDRs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			blockedNets = append(blockedNets, n)
		}
	}
}

// isBlockedIP reports whether an IP is in any SSRF-blocked range.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true // refuse ambiguous
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// SafeDialContext wraps net.Dialer to reject SSRF-blocked destinations
// AFTER DNS resolution — an attacker can't bypass by using a hostname
// that resolves to 127.0.0.1.
func SafeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	// Resolve all A/AAAA records; reject if ANY is blocked
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("webfetch: no IPs for %q", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip.IP) {
			return nil, fmt.Errorf("%w: host=%q ip=%s", ErrSSRFBlocked, host, ip.IP)
		}
	}
	d := &net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

var (
	htmlStripper  = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	spaceStripper = regexp.MustCompile(`\s+`)
)

// WebFetchTool menyediakan kemampuan fetch halaman web dan API lewat HTTP(S).
type WebFetchTool struct {
	client       *http.Client
	maxBodyBytes int
}

type webFetchArgs struct {
	URL string `json:"url" validate:"required"`
}

// NewSafeHTTPClient mengembalikan HTTP client yang memiliki proteksi SSRF,
// membatasi resolve DNS ke internal IP, dan mencegah redirect evasion.
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext:           SafeDialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			host := req.URL.Hostname()
			if host == "" {
				return nil
			}
			if ip := net.ParseIP(host); ip != nil {
				if isBlockedIP(ip) {
					return fmt.Errorf("%w: redirect target (literal IP)", ErrSSRFBlocked)
				}
				return nil
			}
			ips, err := net.LookupIP(host)
			if err != nil {
				return nil
			}
			for _, ip := range ips {
				if isBlockedIP(ip) {
					return fmt.Errorf("%w: redirect domain %q resolves to blocked %s", ErrSSRFBlocked, host, ip)
				}
			}
			return nil
		},
	}
	return client
}

func NewWebFetchTool() *WebFetchTool {
	// Gemini audit BUG_2.MD Bug 1 fix: install SSRF-safe transport.
	client := NewSafeHTTPClient(20 * time.Second)
	return &WebFetchTool{
		client:       client,
		maxBodyBytes: 64 * 1024,
	}
}

// Definition mengembalikan definisi webfetch tool yang terlihat oleh model.
func (t *WebFetchTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "webfetch",
		Description: "Fetch a web page or API response over HTTP(S).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Absolute http or https URL.",
				},
			},
			"required": []string{"url"},
		},
	}
}

// Execute menjalankan satu pemanggilan webfetch tool.
func (t *WebFetchTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args webFetchArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode webfetch arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("validation failed: %w", err)
	}

	parsedURL, err := url.Parse(args.URL)
	if err != nil {
		return Result{}, fmt.Errorf("parse url %q: %w", args.URL, err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		edu := braindb.GetEducationalError(invocation.WorkingDir, "ERR_UNSUPPORTED_SCHEME", parsedURL.Scheme)
		return Result{}, fmt.Errorf("%s", edu)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "flowork/0.1")

	resp, err := t.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrSSRFBlocked) {
			return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(invocation.WorkingDir, "ERR_SSRF_BLOCKED", args.URL))
		}
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(invocation.WorkingDir, "ERR_NETWORK_ERROR", args.URL, err.Error()))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(t.maxBodyBytes+1)))
	if err != nil {
		return Result{}, fmt.Errorf("read response body: %w", err)
	}

	truncated := len(body) > t.maxBodyBytes
	if truncated {
		body = body[:t.maxBodyBytes]
	}

	contentType := resp.Header.Get("Content-Type")
	output := normalizeFetchedContent(string(body), contentType)
	metadata := map[string]any{
		"url":          parsedURL.String(),
		"status":       resp.Status,
		"content_type": contentType,
		"truncated":    truncated,
	}

	if resp.StatusCode >= 400 {
		return Result{
			Output:   output,
			Metadata: metadata,
		}, fmt.Errorf("%s", braindb.GetEducationalError(invocation.WorkingDir, "ERR_HTTP_ERROR", resp.Status, args.URL))
	}

	return Result{
		Output:   output,
		Metadata: metadata,
	}, nil
}

// normalizeFetchedContent melakukan pembersihan minimal pada hasil fetch agar lebih mudah dikonsumsi model.
func normalizeFetchedContent(body string, contentType string) string {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		body = htmlStripper.ReplaceAllString(body, " ")
		body = spaceStripper.ReplaceAllString(body, " ")
		body = strings.TrimSpace(body)
	}
	if body == "" {
		return "(empty response body)"
	}
	return body
}
