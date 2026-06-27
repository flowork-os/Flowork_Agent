// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// ratelimit_track_ext.go — SEAM (NON-frozen, sibling, DELETABLE). SADAR-KUOTA langganan.
//
// Insight (cara Claude Code kerja): Anthropic balikin SISA KUOTA langganan di header response
// TIAP call — `anthropic-ratelimit-unified-5h-utilization` (0..1) + reset + 7d. Claude Code
// BACA ini → tau kapan deket limit → ga nabrak. Flowork dulu BUANG → baru sadar pas 429.
// File ini: baca header itu → 1 STATE SHARE (semua agent lewat dispatcher = nol duplikat) →
// expose helper + handler. Override hook afterAnthropicResponse (POLA B no-op) lewat init().
package router

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimitState — utilisasi kuota langganan (1 sumber kebenaran, share semua agent).
type RateLimitState struct {
	Util5h      float64 `json:"util_5h"`      // 0..1 (window 5-jam)
	Reset5h     string  `json:"reset_5h"`     // dari header, apa adanya
	Surpassed5h bool    `json:"surpassed_5h"` // udah lewat ambang
	Util7d      float64 `json:"util_7d"`      // 0..1 (window 7-hari)
	Reset7d     string  `json:"reset_7d"`
	FallbackPct float64 `json:"fallback_pct"` // % kuota fallback kepake
	Seen        bool    `json:"seen"`         // pernah dapet header (provider langganan)
	UpdatedAt   string  `json:"updated_at"`   // RFC3339 terakhir update
}

var (
	rlMu    sync.RWMutex
	rlState RateLimitState
)

func init() { afterAnthropicResponse = trackAnthropicRateLimit }

// trackAnthropicRateLimit — parse header anthropic-ratelimit-unified-* → update state share.
// Dipanggil tiap response (incl 429). Header ga ada (provider non-langganan) → diabaikan.
func trackAnthropicRateLimit(h http.Header) {
	u5 := h.Get("anthropic-ratelimit-unified-5h-utilization")
	if u5 == "" {
		return
	}
	rlMu.Lock()
	defer rlMu.Unlock()
	rlState.Seen = true
	rlState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if v, err := strconv.ParseFloat(u5, 64); err == nil {
		rlState.Util5h = normUtil(v)
	}
	rlState.Reset5h = h.Get("anthropic-ratelimit-unified-5h-reset")
	rlState.Surpassed5h = rlTruthy(h.Get("anthropic-ratelimit-unified-5h-surpassed-threshold"))
	if v, err := strconv.ParseFloat(h.Get("anthropic-ratelimit-unified-7d-utilization"), 64); err == nil {
		rlState.Util7d = normUtil(v)
	}
	rlState.Reset7d = h.Get("anthropic-ratelimit-unified-7d-reset")
	if v, err := strconv.ParseFloat(h.Get("anthropic-ratelimit-unified-fallback-percentage"), 64); err == nil {
		rlState.FallbackPct = v
	}
}

// normUtil — header kadang 0..1, kadang 0..100 → normalisasi 0..1.
func normUtil(v float64) float64 {
	if v > 1.5 {
		return v / 100
	}
	return v
}

func rlTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "y":
		return true
	}
	return false
}

// RateLimitSnapshot — baca state share (thread-safe). Buat GUI + throttle.
func RateLimitSnapshot() RateLimitState {
	rlMu.RLock()
	defer rlMu.RUnlock()
	return rlState
}

// SubscriptionNearLimit — TRUE kalau kuota 5-jam mepet (>=0.95) ATAU surpassed. Buat throttle
// proaktif: pas mepet, lompat fallback lebih awal alih-alih nabrak 429 + retry-storm.
func SubscriptionNearLimit() bool {
	st := RateLimitSnapshot()
	return st.Seen && (st.Surpassed5h || st.Util5h >= 0.95)
}

// RateLimitHandler — GET /api/router/ratelimit → state kuota (di-wire di package main).
func RateLimitHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(RateLimitSnapshot())
}
