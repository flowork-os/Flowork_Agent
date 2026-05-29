// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 7 phase 2 — normalize router URL ke base only (strip
//   path/query/fragment). Bug fix: per-agent kv.router_url biasanya store
//   full endpoint (mis. `http://127.0.0.1:2402/v1/chat/completions`) yang
//   bikin compose `/api/...` jadi 404. Locked routerclient.go ngga di-
//   modify — extend via helper baru di sini.
//
// normalize.go — Section 7 phase 2: BaseURL normalizer + convenience ctor.

package routerclient

import (
	"net/url"
	"strings"
)

// NormalizeBaseURL — strip path/query/fragment, keep scheme+host+port. Aman
// untuk URL yang sebagian besar agent simpan sebagai endpoint full (kompat
// historical state).
//
// Cara kerja:
//   - URL kosong → return apa adanya (caller fallback ke DefaultRouterURL).
//   - Invalid (gagal parse) → return apa adanya (caller akan whitelist-reject).
//   - Valid → scheme + "://" + host[:port].
func NormalizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

// NewFromAgentURL — convenience ctor untuk caller yang dapet routerURL
// dari per-agent kv (might be full endpoint). Normalize dulu → forward
// ke New (host whitelist + default fallback).
func NewFromAgentURL(routerURL string) *Client {
	return New(NormalizeBaseURL(routerURL))
}
