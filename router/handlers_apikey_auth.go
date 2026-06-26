// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: API Keys → dok lock/gui/API Keys.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

func apiKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isV1Path(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ip := r.RemoteAddr
		if host, _, e := net.SplitHostPort(ip); e == nil {
			ip = host
		}
		r = r.WithContext(router.WithClientIP(r.Context(), ip))

		if aid := strings.TrimSpace(r.Header.Get("X-Agent-ID")); aid != "" {
			r = r.WithContext(router.WithAgentID(r.Context(), aid))
		}
		d, err := store.Open()
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		settings, _ := store.LoadSettings(d)
		requireKey := settings != nil && settings.RequireApiKey

		if settings != nil && settings.Budget.Enforce {
			if msg := globalBudgetExceeded(d, settings.Budget); msg != "" {
				writeAPIKeyError(w, http.StatusTooManyRequests, msg)
				return
			}
		}

		token := extractAPIKey(r)
		if token == "" || !strings.HasPrefix(token, apiKeyPrefix) {

			if requireKey {
				writeAPIKeyError(w, http.StatusUnauthorized, "missing API key — send 'Authorization: Bearer flr_...'")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		key, _ := store.VerifyAPIKey(d, token)
		if key == nil {

			if requireKey {
				writeAPIKeyError(w, http.StatusUnauthorized, "invalid or revoked API key")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if msg := capExceeded(d, key); msg != "" {
			writeAPIKeyError(w, http.StatusTooManyRequests, msg)
			return
		}

		next.ServeHTTP(w, r.WithContext(router.WithAPIKey(r.Context(), key)))
	})
}

func isV1Path(p string) bool {
	return strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/v1beta/")
}

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

func writeAPIKeyError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    "authentication_error",
			"message": msg,
		},
	})
}
