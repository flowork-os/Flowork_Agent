// Package walletalert monitors OpenRouter credit balance and pushes a
// Telegram alert when it drops below a configurable threshold.
//
// Designed to run as a background goroutine inside any daemon.
// Does NOT perform auto-top-up (no programmatic API for that) — instead
// it alerts Ayah so he can top up manually via OpenRouter dashboard.
//
// BUG FIX 2026-05-01 (Ayah report: false-positive "saldo 0" alert padahal
// dashboard $7.09): sebelumnya cuma panggil /api/v1/auth/key dan hitung
// `Limit - Usage`. Untuk paid tier dengan no per-request limit, `Limit=null`
// (decode jadi 0) → `remaining = 0 - 22.88 = -22.88` → trigger alert "saldo 0"
// padahal real saldo $7.09 dari top-up. Sekarang delegate ke
// `nightwatch.CheckOpenRouterBalance()` yang panggil 2 endpoint
// (/auth/key + /credits) dan compute `total_credits - total_usage` benar.
//
// Env vars:
//
//	FLOWORK_WALLET_ALERT_USD   — threshold in USD, default "2.00"
//	FLOWORK_WALLET_CHECK_MINS  — check interval in minutes, default "60"
//	FLOWORK_TG_PUSH_PORT       — Telegram push port, default "8900"
package walletalert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/nightwatch"
	"github.com/teetah2402/flowork/internal/safeclient"
)

const (
	defaultThresholdUSD = 2.0
	defaultIntervalMins = 60
)

// Monitor runs a background goroutine that checks OpenRouter balance
// and pushes a Telegram alert when below threshold.
// Call as: go walletalert.Monitor(ctx, workspace)
func Monitor(ctx context.Context) {
	threshold := defaultThresholdUSD
	if v := os.Getenv("FLOWORK_WALLET_ALERT_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}
	intervalMins := defaultIntervalMins
	if v := os.Getenv("FLOWORK_WALLET_CHECK_MINS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			intervalMins = n
		}
	}

	ticker := time.NewTicker(time.Duration(intervalMins) * time.Minute)
	defer ticker.Stop()

	var lastAlertDay string // track last alert day to avoid spam

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkAndAlert(ctx, threshold, &lastAlertDay)
		}
	}
}

func checkAndAlert(ctx context.Context, threshold float64, lastAlertDay *string) {
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" &&
		strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
		return
	}

	// Delegate ke nightwatch shared function — single source of truth.
	// Panggil 2 endpoint (/auth/key + /credits), compute remaining benar
	// dari `total_credits - total_usage` (bukan `Limit - Usage` yang salah
	// untuk paid tier dengan no per-request limit).
	info, err := nightwatch.CheckOpenRouterBalance()
	if err != nil {
		// Network fail / API down — non-fatal, retry next tick. Log ke stderr
		// supaya bisa diagnose kalau alert seharusnya fire tapi ngga muncul.
		fmt.Fprintf(os.Stderr, "walletalert: balance check fail: %v\n", err)
		return
	}

	// Skip check for unlimited tier (no concept of "remaining")
	if info.Unlimited && info.TotalCredits == 0 {
		return
	}
	if info.IsFree {
		return
	}

	if info.Remaining > threshold {
		return
	}

	// Alert once per day max
	today := time.Now().Format("2006-01-02")
	if *lastAlertDay == today {
		return
	}
	*lastAlertDay = today

	msg := fmt.Sprintf(
		"⚠️ *[WALLET ALERT]* Saldo OpenRouter menipis!\n\n"+
			"💳 Sisa: *$%.2f USD*\n"+
			"📉 Sudah dipakai: *$%.2f USD*\n"+
			"💰 Total top-up: *$%.2f USD*\n"+
			"🔴 Threshold: *$%.2f USD*\n\n"+
			"Segera top-up di https://openrouter.ai/credits agar AI tidak mati.",
		info.Remaining, info.Usage, info.TotalCredits, threshold,
	)
	pushTelegram(msg)
	fmt.Fprintf(os.Stderr, "walletalert: ALERT — remaining $%.2f < threshold $%.2f (total_credits=$%.2f, usage=$%.2f)\n",
		info.Remaining, threshold, info.TotalCredits, info.Usage)
}

// BUG-A14/M04 fix (rc112): http.Post tanpa timeout = hanging goroutine
// kalau flowork-telegram tarpit/hang. Default http.Client tidak punya
// Timeout sehingga 1 push macet bisa menahan FD + goroutine selamanya.
var walletPushClient = safeclient.NewClient(5 * time.Second)

func pushTelegram(text string) {
	port := os.Getenv("FLOWORK_TG_PUSH_PORT")
	if port == "" {
		port = "8900"
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	resp, err := walletPushClient.Post(
		"http://127.0.0.1:"+port+"/push",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "walletalert: push failed: %v\n", err)
		return
	}
	resp.Body.Close()
}
