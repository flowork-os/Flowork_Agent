// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

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

func localAIAutostartEnabled() bool {
	localAIAutostartMu.RLock()
	defer localAIAutostartMu.RUnlock()
	return localAIAutostartOn
}

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

	on := strings.TrimSpace(os.Getenv("FLOWORK_LOCALAI_AUTOSTART")) == "1"
	localAIAutostartMu.Lock()
	localAIAutostartOn = on
	localAIAutostartMu.Unlock()
	_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES ('localai:autostart', ?, datetime('now'))
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
		map[bool]string{true: "true", false: "false"}[on])
}

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
