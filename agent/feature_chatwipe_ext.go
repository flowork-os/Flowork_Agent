// feature_chatwipe_ext.go — WIPE SESI KOLONI dari tombol 🗑 HUD (owner 2026-07-10). NON-FROZEN.
//
// POST /api/chat/wipe {agent} (default mr-flow) → DELETE isi tabel `interactions` di DB
// per-agent (agentdb). PRESISI: cuma riwayat percakapan/working-set yang dihapus —
// decisions, mistakes, dan brain ROUTER (memori penting) TIDAK disentuh. Beda dengan
// /api/agents/db/reset yang nuklir (hapus seluruh file DB). Session-gated (bukan public).
// Hapus file ini → fitur mati, koloni utuh.
package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"flowork-gui/internal/agentdb"

	_ "modernc.org/sqlite"
)

var chatWipeID = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

func chatWipeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Agent string `json:"agent"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	agent := strings.TrimSpace(body.Agent)
	if agent == "" {
		agent = "mr-flow"
	}
	if !chatWipeID.MatchString(agent) {
		http.Error(w, `{"error":"invalid agent id"}`, http.StatusBadRequest)
		return
	}
	path := agentdb.Resolve(agent, "")
	if _, err := os.Stat(path); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "note": "db belum ada"})
		return
	}
	d, err := sql.Open("sqlite", path)
	if err != nil {
		http.Error(w, `{"error":"open db"}`, http.StatusInternalServerError)
		return
	}
	defer d.Close()
	res, err := d.Exec(`DELETE FROM interactions`)
	if err != nil {
		http.Error(w, `{"error":"wipe: `+strings.ReplaceAll(err.Error(), `"`, `'`)+`"}`, http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "agent": agent, "deleted": n})
}

func init() {
	RegisterFeature(Feature{Name: "chat-wipe", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/chat/wipe", chatWipeHandler)
	}})
}
