// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Chat (auto-capture belajar) → dok lock/gui/Chat.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/recorder"
	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/safego"
	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	learnAutoRecordMu sync.RWMutex
	learnAutoRecord   bool
)

func learnAutoCaptureEnabled() bool {
	learnAutoRecordMu.RLock()
	defer learnAutoRecordMu.RUnlock()
	return learnAutoRecord
}

func loadLearnCaptureState() {
	d, err := store.Open()
	if err != nil {
		return
	}
	var v string
	if err := d.QueryRow(`SELECT v FROM kv WHERE k = 'learn:autocapture'`).Scan(&v); err == nil {
		learnAutoRecordMu.Lock()
		learnAutoRecord = v == "true"
		learnAutoRecordMu.Unlock()
	}
}

func learnCaptureToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": learnAutoCaptureEnabled()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	learnAutoRecordMu.Lock()
	learnAutoRecord = body.Enabled
	learnAutoRecordMu.Unlock()
	if d, e := store.Open(); e == nil {
		_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES ('learn:autocapture', ?, datetime('now'))
			ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
			map[bool]string{true: "true", false: "false"}[body.Enabled])
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": body.Enabled})
}

func captureLearningRecording(resp *router.OpenAIResponse, req router.OpenAIRequest, r *http.Request) {
	if !learnAutoCaptureEnabled() || resp == nil || len(resp.Choices) == 0 {
		return
	}
	respText := strings.TrimSpace(resp.Choices[0].Message.Content)
	if respText == "" {
		return
	}
	model := resp.Model
	if model == "" {
		model = req.Model
	}
	if strings.Contains(strings.ToLower(model), "flowork") {
		log.Printf("learnRecord: skip model lokal %s (cuma experience model KUAT yg di-capture)", model)
		return
	}
	agent := detectLearnClient(r.UserAgent())
	safego.GoLabel("learnRecord", func() {
		if _, err := recorder.Save(context.Background(), recorder.RecordOpts{
			Model:        model,
			RequestBody:  req,
			ResponseText: respText,
			InputTokens:  int64(resp.Usage.PromptTokens),
			OutputTokens: int64(resp.Usage.CompletionTokens),
			BuildPass:    -1,
			Agent:        agent,
		}); err == nil {
			log.Printf("learnRecord: captured model=%s agent=%s (%d chars) → recordings", model, agent, len(respText))
		}
	})
}

func detectLearnClient(ua string) string {
	u := strings.ToLower(ua)
	switch {
	case strings.Contains(u, "vscode") || strings.Contains(u, "vs code") || strings.Contains(u, "code/"):
		return "vscode"
	case strings.Contains(u, "claude"):
		return "claude-cli"
	case strings.Contains(u, "cursor"):
		return "cursor"
	default:
		return "router"
	}
}
