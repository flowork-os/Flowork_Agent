// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package builtins

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"flowork-gui/internal/tools"
)

const (
	webFetchMaxBytes = 1 * 1024 * 1024
	webFetchTimeout  = 30 * time.Second
)

var blockedCIDRs []*net.IPNet

func init() {
	for _, cidr := range []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	} {
		if _, n, err := net.ParseCIDR(cidr); err == nil {
			blockedCIDRs = append(blockedCIDRs, n)
		}
	}
}

func isBlockedIP(ip net.IP) bool {
	for _, n := range blockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

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

	client := &http.Client{
		Timeout:   webFetchTimeout,
		Transport: &http.Transport{DialContext: safeFetchDial},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {

			if _, verr := validateURL(req.URL.String()); verr != nil {
				return fmt.Errorf("redirect blocked: %w", verr)
			}

			req.Header.Del("Authorization")
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return tools.Result{}, fmt.Errorf("build request: %w", err)
	}
	// Header gaya browser realistis biar ga keblokir anti-bot (banyak situs nolak UA bot →403).
	// Contek Claude WebFetch: Accept markdown/html + UA wajar + Accept-Language. Override-able via env.
	ua := strings.TrimSpace(os.Getenv("FLOWORK_WEBFETCH_UA"))
	if ua == "" {
		ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	}
	httpReq.Header.Set("User-Agent", ua)
	httpReq.Header.Set("Accept", "text/markdown, text/html, application/xhtml+xml, */*")
	httpReq.Header.Set("Accept-Language", "id-ID, id;q=0.9, en;q=0.8")

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
	bodyStr := string(bodyBytes)
	// CONTEK CLAUDE WebFetch: HTML → teks BERSIH (bukan HTML mentah yg bikin LLM keflood/muntah
	// → error). Reuse stripTags (web_research.go, sepaket). Non-HTML (json/text) = apa adanya.
	if strings.Contains(strings.ToLower(ctype), "html") {
		bodyStr = stripTags(bodyStr)
	}
	return tools.Result{Output: map[string]any{
		"url":          u.String(),
		"status":       resp.StatusCode,
		"content_type": ctype,
		"body":         bodyStr,
		"truncated":    truncated,
		"size_bytes":   len(bodyBytes),
	}}, nil
}
