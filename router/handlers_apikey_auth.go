// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — HTTP handler.

// Inbound API-Key Gate for /v1 (client auth).

package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/store"
)

const apiKeyPrefix = "flr_"

// apiKeyMiddleware gates /v1 + /v1beta with flow_router API keys. All other
// paths pass straight through to the next handler.
func apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isV1Path(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		// Stash the client IP (host only — the port varies per connection) so the
		// dispatcher can do sticky proxy affinity.
		ip := r.RemoteAddr
		if host, _, e := net.SplitHostPort(ip); e == nil {
			ip = host
		}
		r = r.WithContext(router.WithClientIP(r.Context(), ip))
		// #3 scoped-instinct (RI-5): stash caller agent id (X-Agent-ID header) ke ctx SEBELUM
		// cabang auth → kena SEMUA jalur (keyed + keyless local). Kosong = anonim/external (fails-open).
		if aid := strings.TrimSpace(r.Header.Get("X-Agent-ID")); aid != "" {
			r = r.WithContext(router.WithAgentID(r.Context(), aid))
		}
		d, err := store.Open()
		if err != nil {
			next.ServeHTTP(w, r) // fail-open on store error (never lock out local use)
			return
		}
		settings, _ := store.LoadSettings(d)
		requireKey := settings != nil && settings.RequireApiKey

		// Global budget ceiling — applies to ALL /v1 traffic (keyed or not),
		// only when explicitly enforced (so default stays unlimited).
		if settings != nil && settings.Budget.Enforce {
			if msg := globalBudgetExceeded(d, settings.Budget); msg != "" {
				writeAPIKeyError(w, http.StatusTooManyRequests, msg)
				return
			}
		}

		token := extractAPIKey(r)
		if token == "" || !strings.HasPrefix(token, apiKeyPrefix) {
			// No flow_router key presented.
			if requireKey {
				writeAPIKeyError(w, http.StatusUnauthorized, "missing API key — send 'Authorization: Bearer flr_...'")
				return
			}
			next.ServeHTTP(w, r) // open local mode
			return
		}

		key, _ := store.VerifyAPIKey(d, token)
		if key == nil {
			// Presented an flr_ key but it is invalid/revoked.
			if requireKey {
				writeAPIKeyError(w, http.StatusUnauthorized, "invalid or revoked API key")
				return
			}
			next.ServeHTTP(w, r) // not mandatory → treat as anonymous
			return
		}

		// Valid key → enforce its caps before dispatch.
		if msg := capExceeded(d, key); msg != "" {
			writeAPIKeyError(w, http.StatusTooManyRequests, msg)
			return
		}

		// Attach to context for the dispatcher (usage attribution + scope).
		next.ServeHTTP(w, r.WithContext(router.WithAPIKey(r.Context(), key)))
	})
}

func isV1Path(p string) bool {
	return strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/v1beta/")
}

// extractAPIKey reads the client key from Authorization: Bearer or x-api-key.
// (Cookies are deliberately ignored here — those are GUI sessions, not keys.)
func extractAPIKey(r *http.Request) string {
	if v := r.Header.Get("x-api-key"); v != "" {
		return strings.TrimSpace(v)
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// capExceeded returns a non-empty reason when the key is over a configured cap.
// Caps of 0 mean unlimited. Spend is summed from the usageDaily aggregate, so
// enforcement is a soft cap (the request that crosses it still completes; the
// next one is blocked) — standard gateway behaviour.
func capExceeded(d *sql.DB, key *store.APIKey) string {
	if key.DailyCapUsd > 0 {
		today := time.Now().UTC().Format("2006-01-02")
		if spent, err := store.SpendSince(d, key.ID, today); err == nil && spent >= key.DailyCapUsd {
			return fmt.Sprintf("daily cap reached ($%.2f / $%.2f)", spent, key.DailyCapUsd)
		}
	}
	if key.MonthlyCapUsd > 0 {
		now := time.Now().UTC()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		if spent, err := store.SpendSince(d, key.ID, monthStart); err == nil && spent >= key.MonthlyCapUsd {
			return fmt.Sprintf("monthly cap reached ($%.2f / $%.2f)", spent, key.MonthlyCapUsd)
		}
	}
	return ""
}

// globalBudgetExceeded returns a non-empty reason when total spend (all keys +
// anonymous) is over a configured global cap. Caps of 0 = unlimited. WarnUsd
// (when set and crossed but under the cap) emits a server-log warning only.
func globalBudgetExceeded(d *sql.DB, b store.Budget) string {
	today := time.Now().UTC().Format("2006-01-02")
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")

	daySpend, _ := store.TotalSpendSince(d, today)
	if b.DailyCapUsd > 0 && daySpend >= b.DailyCapUsd {
		return fmt.Sprintf("global daily budget reached ($%.2f / $%.2f)", daySpend, b.DailyCapUsd)
	}
	if b.MonthlyCapUsd > 0 {
		monthSpend, _ := store.TotalSpendSince(d, monthStart)
		if monthSpend >= b.MonthlyCapUsd {
			return fmt.Sprintf("global monthly budget reached ($%.2f / $%.2f)", monthSpend, b.MonthlyCapUsd)
		}
	}
	if b.WarnUsd > 0 && daySpend >= b.WarnUsd {
		log.Printf("flow_router budget WARNING: today's spend $%.2f crossed warn threshold $%.2f", daySpend, b.WarnUsd)
	}
	return ""
}

// writeAPIKeyError emits an OpenAI-shaped error envelope.
func writeAPIKeyError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    "authentication_error",
			"message": msg,
		},
	})
}
