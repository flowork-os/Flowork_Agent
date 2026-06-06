// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: http.fetch SSRF boundary. The dial-time IP guard (ssrfSafeClient) is the
//   authoritative defense — it covers redirects, DNS rebinding, private ranges and
//   the cloud-metadata endpoint. Weakening it re-opens module→internal SSRF.
package loket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// ssrfSafeClient is the ONLY client http.fetch uses. Its enforcement lives at the
// socket layer (Dialer.Control): every connect — the first hop AND every redirect
// hop — is checked against the resolved IP, so the guard cannot be bypassed by a
// redirect to an internal host, by DNS rebinding (a public name that resolves to a
// private IP), or by an alternate textual IP encoding. The hostname pre-check in
// httpFetch is a fast, friendly early-out; this dial guard is the real boundary.
var ssrfSafeClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					host = address
				}
				ip := net.ParseIP(host)
				if ip != nil && isBlockedIP(ip) {
					return fmt.Errorf("blocked address %s (loopback/private/link-local/metadata)", ip)
				}
				return nil
			},
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	},
	CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	},
}

// isBlockedIP reports whether an IP a module is about to connect to is off-limits:
// loopback, the unspecified address, link-local (incl. 169.254.169.254 — the cloud
// metadata service that hands out credentials), and RFC1918/ULA private networks.
// A module's outbound HTTP is for the public web; reaching the host's own internals
// or the local network is the SSRF the guard exists to stop.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsPrivate()
}

// httpFetch performs an outbound HTTP request on a module's behalf. It is a
// GrantOwner capability: a module gets it only if it declares "http.fetch" in its
// loket.json and the owner approves. A module's own raw network is scoped to the
// loket endpoint, so this is the ONLY way it reaches the outside web — through the
// kernel, which can govern it. Args: {url, method?, headers?, body?, timeout_ms?}.
func httpFetch(ctx context.Context, _ string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		URL       string            `json:"url"`
		Method    string            `json:"method"`
		Headers   map[string]string `json:"headers"`
		Body      string            `json:"body"`
		TimeoutMs int               `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.URL == "" {
		return nil, fmt.Errorf("http.fetch: url required")
	}
	if a.Method == "" {
		a.Method = http.MethodGet
	}
	to := time.Duration(a.TimeoutMs) * time.Millisecond
	if to <= 0 || to > 120*time.Second {
		to = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	var body io.Reader
	if a.Body != "" {
		body = bytes.NewReader([]byte(a.Body))
	}
	req, err := http.NewRequestWithContext(cctx, a.Method, a.URL, body)
	if err != nil {
		return nil, err
	}
	// SSRF guard (fast early-out): block obvious loopback names up front for a clear
	// error. The authoritative enforcement is ssrfSafeClient's dial guard below, which
	// also covers redirects, DNS rebinding, private ranges and the metadata endpoint.
	if isLoopbackHost(req.URL.Hostname()) {
		return nil, fmt.Errorf("http.fetch: loopback/local hosts are not allowed")
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	resp, err := ssrfSafeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http.fetch: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // cap 8 MiB
	return json.Marshal(map[string]any{"status": resp.StatusCode, "body": string(respBody)})
}

// isLoopbackHost reports whether a host points at the local machine.
func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return strings.HasPrefix(host, "127.") || strings.HasSuffix(host, ".localhost")
}
