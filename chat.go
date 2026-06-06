// chat.go — CHANNEL HTTP/CLI (roadmap Channels, langkah AMAN). mr-flow UDAH
// channel-agnostic: rpc handle_message (route/classify/chat, parity Telegram).
// Endpoint ini = transport ke-2 (web/CLI) yang INVOKE core itu — TANPA nyentuh
// daemon Telegram LIVE (additive, nol risiko bot).
//
//	POST /api/chat {text, user?}  → invoke mr-flow → {reply}
//
// Juga = TEST HARNESS doktrin ("chat-debug lewat jalur SAMA Telegram") — respons
// identik sama yang user dapet di Telegram (jalur rpc mirror daemon). Loopback-only.
//
// CATATAN: ini channel BUILT-IN (belum plugin kind=channel). Telegram-daemon →
// plugin removable = surgery bot LIVE, risiko tinggi → DEFER (lihat ROADMAP_CHANNELS).

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"flowork-gui/internal/kernelhost"
)

// chatHandler — POST /api/chat. Invoke mr-flow (channel-agnostic core) → reply.
func chatHandler(host *kernelhost.Host) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Text string `json:"text"`
			User string `json:"user"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "text required"})
			return
		}
		caller := strings.TrimSpace(body.User)
		if caller == "" {
			caller = "cli:owner"
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		raw, err := host.InvokeAgentMessage(ctx, "mr-flow", body.Text, caller)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		// mr-flow emit JSON {"reply":"..."} (atau {"error":...}). Parse, fallback raw.
		reply := strings.TrimSpace(raw)
		var emitted map[string]any
		if json.Unmarshal([]byte(raw), &emitted) == nil {
			if rv, ok := emitted["reply"].(string); ok {
				reply = rv
			} else if ev, ok := emitted["error"].(string); ok {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": ev})
				return
			}
		}
		tfWriteJSON(w, 0, map[string]any{"reply": reply, "channel": "http", "caller": caller})
	}
}
