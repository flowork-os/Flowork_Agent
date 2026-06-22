// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-29
// Reason: Section 7 Agent (Sync interface ke router) phase 1 DONE +
//   adversarial-audit passed (C1 URL whitelist anti-SSRF, fallback ke
//   DefaultRouterURL kalau attacker set kv.router_url ke external).
//   API stable: New, Client, SubmitMistake, Ping. Phase 2 methods
//   (PullSkill, QueryBrain, retry/circuit-breaker) → tambah method
//   baru di file ini OK, JANGAN modify existing.
// 2026-06-22 (owner-approved, audit security): FIX SSRF bypass di isAllowedRouterURL —
//   parse net/url + tolak userinfo (`user@host`). Parser string lama bisa di-bypass
//   (`http://127.0.0.1:80@evil.com` lolos → dial evil.com → exfil brain). Re-lock.
//
// Package routerclient — HTTP client wrapper untuk agent↔router communication.
//
// PURPOSE:
//   Komunikasi tipe-safe Agent → Router. Phase 1: SubmitMistake (push
//   mistakes promotion). Phase 2: PullSkill (skill catalog browse),
//   QueryBrain (drawer retrieve).
//
// SECURITY:
//   - Router URL per-agent dari `kv.router_url` (default fallback to
//     `http://127.0.0.1:2402`).
//   - HTTP client timeout 30 detik (anti-stuck di slow router).
//   - Response status code check + body decode error → return error to caller.
//
// CALLER:
//   - Kernel-side promote cron (Section 7 phase 1) — periodic push.
//   - Future: WASM agent via host capability wrapper (defer phase 2).
//
// Source: Flowork_Agent/roadmap.md Section 7.

package routerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultRouterURL — fallback kalau kv.router_url ngga set.
const DefaultRouterURL = "http://127.0.0.1:2402"

// DefaultTimeout — HTTP request timeout.
const DefaultTimeout = 30 * time.Second

// Client — agent → router HTTP wrapper.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// allowedHosts — whitelist host yang router URL boleh point ke. Default
// localhost (127.0.0.1, localhost). Operator dengan multi-host setup
// extend via env atau config future. Anti SSRF / data exfil ke attacker
// controlled host.
var allowedHosts = map[string]struct{}{
	"127.0.0.1": {},
	"localhost": {},
	"0.0.0.0":   {},
}

// New returns a Client siap pakai. URL kosong → DefaultRouterURL.
// Audit fix C1: validate URL host against whitelist — kalau attacker
// (atau buggy config) set kv.router_url ke external, fallback ke default.
func New(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultRouterURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	// Validate host whitelist. Parse URL → extract host (strip port).
	if !isAllowedRouterURL(baseURL) {
		baseURL = DefaultRouterURL
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: DefaultTimeout},
	}
}

// isAllowedRouterURL — return true kalau baseURL host ada di allowedHosts.
// Defense in depth — cegah kv.router_url di-set ke external attacker.
//
// ⚠️ 2026-06-22 (owner-approved, audit security): parse pakai net/url + TOLAK userinfo.
// Parser string manual lama bisa di-bypass: `http://127.0.0.1:80@evil.com` lolos whitelist
// (last-colon-split nyisain "127.0.0.1") tapi Go nge-dial `evil.com` (userinfo) → exfil
// brain. url.Hostname() buang userinfo+port dgn bener; tolak kalau ada u.User (anti-bypass).
func isAllowedRouterURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil || u.User != nil { // userinfo (user:pass@host) = vektor bypass → TOLAK
		return false
	}
	host := u.Hostname() // strip port + userinfo dgn benar (beda dari parser string manual)
	if host == "" {
		return false
	}
	_, ok := allowedHosts[host]
	return ok
}

// SubmitMistakeReq — payload buat POST /api/mistakes/submit.
// Mirror schema Router brain mistakes_journal field.
type SubmitMistakeReq struct {
	AgentID  string `json:"agent_id"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	HitCount int64  `json:"hit_count"`
}

// SubmitMistakeResp — return dari Router. ID = mistakes_journal.id global,
// Added = true kalau insert baru, false kalau upsert existing.
type SubmitMistakeResp struct {
	ID    int64  `json:"id"`
	Added bool   `json:"added"`
	Error string `json:"error,omitempty"`
}

// SubmitMistake — POST /api/mistakes/submit. Push mistake hit_count≥3 dari
// agent ke Router brain global tier. Return resp.ID (router-side row id)
// supaya caller bisa simpan di `mistakes_local.promoted_to_id`.
func (c *Client) SubmitMistake(ctx context.Context, req SubmitMistakeReq) (SubmitMistakeResp, error) {
	if c == nil {
		return SubmitMistakeResp{}, fmt.Errorf("router client nil")
	}
	url := c.BaseURL + "/api/mistakes/submit"

	bodyJSON, err := json.Marshal(req)
	if err != nil {
		return SubmitMistakeResp{}, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return SubmitMistakeResp{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return SubmitMistakeResp{}, fmt.Errorf("submit mistake: %w", err)
	}
	defer resp.Body.Close()

	// Cap body read 64KB (response kecil — kalau lebih, suspicious).
	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var out SubmitMistakeResp
	if uerr := json.Unmarshal(respBytes, &out); uerr != nil {
		return SubmitMistakeResp{}, fmt.Errorf("decode (status=%d): %w", resp.StatusCode, uerr)
	}
	if resp.StatusCode >= 400 {
		if out.Error == "" {
			out.Error = fmt.Sprintf("router status %d", resp.StatusCode)
		}
		return out, fmt.Errorf("router error: %s", out.Error)
	}
	return out, nil
}

// Ping — quick health check ke router. Lightweight call ke /v1/health atau
// fallback ke base URL — return error kalau ngga reachable.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("router client nil")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("router unhealthy status %d", resp.StatusCode)
	}
	return nil
}
