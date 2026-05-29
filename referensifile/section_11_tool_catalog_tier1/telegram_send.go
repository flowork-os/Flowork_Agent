package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/safeclient"
	"github.com/teetah2402/flowork/internal/silentmode"
)

// TelegramSendTool — kirim pesan ke Telegram Ayah via flowork-telegram push
// endpoint. Bug #2 fix (2026-04-18): sebelumnya agent tidak punya tool
// terintegrasi untuk notify Ayah ke Telegram, harus tanya user webhook URL
// manually. Tool ini hardcode default endpoint flowork-telegram yang running
// lokal di port 8900 (overridable via FLOWORK_TG_PUSH_URL env).
//
// Flowork-telegram binary (cmd/flowork-telegram/main.go) expose POST /push
// yang forward body.text ke Telegram Bot API via TELEGRAM_BOT_TOKEN +
// TELEGRAM_OWNER_IDS whitelist. Jadi tool ini aman: tidak leak API key,
// tidak butuh user interaksi, cuma forward text ke local relay.
//
// Scope intentionally minimal: 1 arg `text`. Kalau butuh markdown atau
// inline keyboard, tambah args opsional di revisi future (YAGNI sekarang).
type TelegramSendTool struct {
	client  *http.Client
}

type telegramSendArgs struct {
	Text string `json:"text" validate:"required"`
}

// NewTelegramSendTool returns tool with default endpoint resolution:
//
//  1. FLOWORK_TG_PUSH_URL env (explicit override)
//  2. http://localhost:8900/push (default when flowork-telegram.exe is running)
//
// Caller tidak perlu pass endpoint — tool resolve sendiri at Execute time
// (env bisa berubah across sessions).
//
// Bug #14 fix: use safeclient instead of plain http.Client to prevent SSRF
// if FLOWORK_TG_PUSH_URL is poisoned (e.g. via chained .env injection).
func NewTelegramSendTool() *TelegramSendTool {
	return &TelegramSendTool{
		client: safeclient.NewClient(10 * time.Second),
	}
}

// resolveURL picks the current push endpoint. Called per-Execute so env
// changes between calls are respected (e.g., owner switches telegram bot
// port mid-session).
func (t *TelegramSendTool) resolveURL() string {
	if v := os.Getenv("FLOWORK_TG_PUSH_URL"); strings.TrimSpace(v) != "" {
		return v
	}
	return "http://localhost:8900/push"
}

func (t *TelegramSendTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "telegram_send",
		Description: `Kirim pesan ke Telegram Ayah (Owner) via flowork-telegram relay.

Gunakan saat:
  - Notify Ayah tentang bug, progress, atau completion penting (owner awareness)
  - Escalate request yang butuh Ayah approval (misal legal gate, commit authorization)
  - Forward error critical yang perlu Ayah lihat segera
  - Confirm task selesai setelah direct request dari Ayah via chat/web UI

Endpoint default: http://localhost:8900/push (flowork-telegram.exe harus running).
Override via FLOWORK_TG_PUSH_URL env kalau Ayah set bot custom.

Tool return: status code + response preview. Error kalau relay down atau network fail.

Aturan:
  - Keep message short (<4000 char). Telegram hard-limit 4096.
  - Tulis sebagai first-person AI ("gw notify...", "tim selesai..."), bukan third-person.
  - Jangan spam: satu tool call per event, bukan per paragraf.
  - Kalau ada link (commit hash, file path), include inline.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Isi pesan Telegram. Max 4000 char recommended.",
				},
			},
			"required": []string{"text"},
		},
	}
}

func (t *TelegramSendTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args telegramSendArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode telegram_send arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	text := strings.TrimSpace(args.Text)
	if text == "" {
		return Result{}, fmt.Errorf("telegram_send: text is required")
	}
	if len(text) > 4000 {
		text = text[:3997] + "..."
	}

	// Silent-hours filter: suppress proactive Telegram pushes during quiet
	// window. Emergency messages (containing DARURAT/URGENT/KRITIS) bypass.
	if silentmode.IsSilent(time.Now()) && !isTelegramEmergency(text) {
		return Result{
			Output: "[silent-hours] Telegram push suppressed — will retry outside quiet window",
		}, nil
	}

	url := t.resolveURL()
	body, _ := json.Marshal(map[string]string{"text": text})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("telegram_send: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// BUG-M17 fix (2026-04-19): saat flowork-telegram daemon DOWN, graceful
	// degradation — return success result dengan warning message (bukan error)
	// supaya autonomous agent gak loop retry ngabisin token. Agent lihat
	// Output jelas daemon offline dan bisa lanjut kerja tanpa telegram.
	resp, err := t.client.Do(req)
	if err != nil {
		return Result{
			Output: fmt.Sprintf("[telegram offline] pesan tidak terkirim — daemon flowork-telegram.exe tidak berjalan di %s (error: %v). Melanjutkan tanpa retry.", url, err),
			Metadata: map[string]any{
				"url":              url,
				"error":            err.Error(),
				"graceful_degrade": true,
			},
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	respStr := strings.TrimSpace(string(respBody))

	metadata := map[string]any{
		"url":         url,
		"status_code": resp.StatusCode,
		"text_length": len(text),
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Result{
			Output:   fmt.Sprintf("Pesan terkirim ke Telegram (HTTP %d). Response: %s", resp.StatusCode, respStr),
			Metadata: metadata,
		}, nil
	}

	return Result{
		Output:   fmt.Sprintf("Telegram relay rejected (HTTP %d): %s", resp.StatusCode, respStr),
		Metadata: metadata,
	}, fmt.Errorf("telegram relay returned HTTP %d", resp.StatusCode)
}

// isTelegramEmergency returns true when the message contains a keyword that
// warrants bypassing silent-hours suppression.
func isTelegramEmergency(text string) bool {
	upper := strings.ToUpper(text)
	for _, kw := range []string{"DARURAT", "URGENT", "KRITIS", "EMERGENCY", "CRITICAL", "BREAK_GLASS"} {
		if strings.Contains(upper, kw) {
			return true
		}
	}
	return false
}
