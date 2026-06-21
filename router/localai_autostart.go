package main

// localai_autostart.go — local-AI (flowork-brain) autostart sebagai SETTING GUI
// (owner 2026-06-21: "AI lokal mau auto-start atau ngak ada setingannya di GUI, env
// dihapus biar gak bingung"). Dulu gate = env FLOWORK_LOCALAI_AUTOSTART. Sekarang kv
// 'localai:autostart' (GUI toggle) = sumber kebenaran. Migrasi SEKALI dari env (kalau
// kv belum di-set & env=1) biar setup lama gak ke-break, abis itu env boleh dihapus.

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	localAIAutostartMu sync.RWMutex
	localAIAutostartOn bool
)

// localAIAutostartEnabled — sumber tunggal autostart local-AI (kv via GUI).
func localAIAutostartEnabled() bool {
	localAIAutostartMu.RLock()
	defer localAIAutostartMu.RUnlock()
	return localAIAutostartOn
}

// loadLocalAIAutostartState — load kv 'localai:autostart' saat boot. Kalau kv belum ada,
// MIGRASI sekali dari env FLOWORK_LOCALAI_AUTOSTART=1 (transisi) lalu persist ke kv.
func loadLocalAIAutostartState() {
	d, err := store.Open()
	if err != nil {
		return
	}
	var v string
	if err := d.QueryRow(`SELECT v FROM kv WHERE k = 'localai:autostart'`).Scan(&v); err == nil {
		localAIAutostartMu.Lock()
		localAIAutostartOn = v == "true"
		localAIAutostartMu.Unlock()
		return
	}
	// kv belum di-set → migrasi sekali dari env (setup lama), lalu persist.
	on := strings.TrimSpace(os.Getenv("FLOWORK_LOCALAI_AUTOSTART")) == "1"
	localAIAutostartMu.Lock()
	localAIAutostartOn = on
	localAIAutostartMu.Unlock()
	_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES ('localai:autostart', ?, datetime('now'))
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
		map[bool]string{true: "true", false: "false"}[on])
}

// localAIAutostartToggleHandler — GET status / POST {enabled:bool} → toggle (GUI). Persist kv.
// Catatan: efek START/STOP llama berlaku saat boot router berikutnya (autostart = boot-time).
func localAIAutostartToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": localAIAutostartEnabled()})
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
	localAIAutostartMu.Lock()
	localAIAutostartOn = body.Enabled
	localAIAutostartMu.Unlock()
	if d, e := store.Open(); e == nil {
		_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES ('localai:autostart', ?, datetime('now'))
			ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
			map[bool]string{true: "true", false: "false"}[body.Enabled])
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": body.Enabled})
}
