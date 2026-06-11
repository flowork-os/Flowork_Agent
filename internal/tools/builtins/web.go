// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// 2026-06-11 (owner-approved security audit, unfreeze→refreeze): webfetch client
//   now uses a Transport.DialContext (safeFetchDial) that re-checks + pins the IP
//   at connect time — closes a DNS-rebinding window left by the one-shot
//   validateURL() resolve. Also covers redirect targets.
// Reason: Section 11 phase 1d (webfetch) DONE. API stable: webfetch
//   tool dengan SSRF defense — scheme whitelist (http/https), hostname
//   resolve + IP CIDR block (127.x loopback, 10.x/172.16-31.x/192.168.x
//   private, 169.254.x metadata, IPv6 ::1/fc00::/fe80::). CheckRedirect
//   re-validate + strip Authorization. Response cap 1MB, timeout 30s.
//   User-Agent identifies Mr.Flow. Phase 1d+ web tools (websearch,
//   webscrape) → tambah file baru, JANGAN modify ini.
//
// web.go — Section 11 phase 1d: webfetch tool.
//
// Tool: webfetch — HTTP GET to arbitrary public URL. Return status,
// headers (subset), body (capped).
//
// SECURITY (SSRF defense):
//   - Scheme whitelist: http, https only.
//   - Hostname resolve + block private/loopback IPs (127.x, 10.x,
//     172.16-31.x, 192.168.x, 169.254.x metadata, fc00::/7, ::1).
//   - Response body cap 1MB.
//   - HTTP timeout 30s.
//   - No follow auth headers across redirect.
//
// CAPABILITY: net:fetch:*

package builtins

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"flowork-gui/internal/tools"
)

const (
	webFetchMaxBytes = 1 * 1024 * 1024 // 1MB response cap
	webFetchTimeout  = 30 * time.Second
)

// blockedCIDRs — private/loopback/metadata IP ranges. SSRF guard.
var blockedCIDRs []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // private
		"172.16.0.0/12",  // private
		"192.168.0.0/16", // private
		"169.254.0.0/16", // link-local + AWS/GCP metadata
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	} {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			blockedCIDRs = append(blockedCIDRs, n)
		}
	}
}

// isBlockedIP — return true kalau IP ada di blocked CIDRs.
func isBlockedIP(ip net.IP) bool {
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// validateURL — parse + scheme check + hostname resolve + IP block check.
// Return error kalau invalid atau pointing ke private network.
func validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http/https (got %q)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("host required")
	}
	// Resolve hostname — kalau direct IP, parse; kalau hostname, lookup.
	ips, lerr := net.LookupIP(host)
	if lerr != nil {
		return nil, fmt.Errorf("dns lookup: %w", lerr)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("ip %s blocked (private/loopback/metadata range)", ip)
		}
	}
	return u, nil
}

// safeFetchDial re-validates the destination IP at connection time and dials the
// exact validated IP (pinned), closing the DNS-rebinding gap that a one-shot
// validateURL() leaves open. The TLS ServerName is derived from the request URL
// host by net/http, not from the dial address, so cert verification still works
// against the original hostname.
func safeFetchDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: webFetchTimeout, KeepAlive: 30 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return nil, fmt.Errorf("ip %s blocked (private/loopback/metadata range)", ip)
		}
		return d.DialContext(ctx, network, addr)
	}
	ips, lerr := net.DefaultResolver.LookupIPAddr(ctx, host)
	if lerr != nil {
		return nil, fmt.Errorf("dns lookup: %w", lerr)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("dns lookup: no addresses for %s", host)
	}
	for _, a := range ips {
		if isBlockedIP(a.IP) {
			return nil, fmt.Errorf("ip %s blocked (private/loopback/metadata range)", a.IP)
		}
	}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// =============================================================================
// webfetch — HTTP GET URL
// =============================================================================

type webFetchTool struct{}

func (webFetchTool) Name() string       { return "webfetch" }
func (webFetchTool) Capability() string { return "net:fetch:*" }
func (webFetchTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "HTTP GET public URL. SSRF guard blocks private/loopback/metadata IPs. Response cap 1MB, timeout 30s.",
		Params: []tools.Param{
			{Name: "url", Type: tools.ParamString, Description: "absolute http(s) URL", Required: true},
		},
		Returns: "{url, status, content_type, body, truncated: bool, size_bytes}",
	}
}

func (webFetchTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	raw, _ := args["url"].(string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tools.Result{}, fmt.Errorf("url required")
	}

	u, err := validateURL(raw)
	if err != nil {
		return tools.Result{}, err
	}

	// Build client dengan timeout + redirect policy yang strip auth.
	// Transport.DialContext re-checks the IP at connect time and pins the dial to
	// the validated address — validateURL() alone resolves once up front, leaving a
	// DNS-rebinding window where the name re-resolves to a private/metadata IP at
	// dial time. Re-checking per-dial also covers redirect targets.
	client := &http.Client{
		Timeout:   webFetchTimeout,
		Transport: &http.Transport{DialContext: safeFetchDial},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Re-validate redirect target untuk SSRF defense — kalau attacker
			// host respond 301 ke private IP, blok.
			if _, verr := validateURL(req.URL.String()); verr != nil {
				return fmt.Errorf("redirect blocked: %w", verr)
			}
			// Strip Authorization header on cross-host redirect.
			req.Header.Del("Authorization")
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return tools.Result{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", "Flowork-Mr.Flow/1.0 (webfetch tool)")

	resp, derr := client.Do(httpReq)
	if derr != nil {
		return tools.Result{}, fmt.Errorf("fetch: %w", derr)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes+1))
	truncated := false
	if len(bodyBytes) > webFetchMaxBytes {
		bodyBytes = bodyBytes[:webFetchMaxBytes]
		truncated = true
	}

	ctype := resp.Header.Get("Content-Type")
	return tools.Result{Output: map[string]any{
		"url":          u.String(),
		"status":       resp.StatusCode,
		"content_type": ctype,
		"body":         string(bodyBytes),
		"truncated":    truncated,
		"size_bytes":   len(bodyBytes),
	}}, nil
}
