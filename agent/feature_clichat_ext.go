// feature_clichat_ext.go — CHAT JALUR RATU (owner 2026-07-10, GUI Mr.Flow). NON-FROZEN sibling.
//
// GUI :1987 punya 2 kanal chat: KOLONI (/api/chat = jalur Telegram) dan RATU·CLI (endpoint ini).
// POST /api/cli-chat {text} → spawn `node <FLOWCLI>/bin/flowcli -p "<text>"` (jalur CLI ASLI,
// persis kayak owner ngetik di terminal — bukan bypass) → balikin stdout sbg {reply}.
//
// Keamanan: TIDAK didaftar di isPublicPath/loopback-allow → kena authMgr.Middleware global
// (wajib sesi login koloni). Path flowcli: env FLOWORK_FLOWCLI_BIN → exe-relative
// (FLowork_os/agent/bin → ../../../FLOWORK/FLOWCLI). Hapus file ini → fitur mati, koloni utuh.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func cliChatBin() (bin string, root string) {
	if p := os.Getenv("FLOWORK_FLOWCLI_BIN"); p != "" {
		return p, filepath.Dir(filepath.Dir(p))
	}
	if exe, err := os.Executable(); err == nil {
		// <...>/FLowork_os/agent/bin/flowork-gui → <...>/Documents/FLOWORK/FLOWCLI
		cand := filepath.Join(filepath.Dir(exe), "..", "..", "..", "FLOWORK", "FLOWCLI")
		if st, err := os.Stat(filepath.Join(cand, "bin", "flowcli")); err == nil && !st.IsDir() {
			return filepath.Join(cand, "bin", "flowcli"), cand
		}
	}
	return "", ""
}

func cliChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		http.Error(w, `{"error":"text required"}`, http.StatusBadRequest)
		return
	}
	bin, root := cliChatBin()
	if bin == "" {
		http.Error(w, `{"error":"flowcli tidak ketemu (set FLOWORK_FLOWCLI_BIN)"}`, http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 290*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", bin, "-p", body.Text)
	cmd.Dir = root
	out, err := cmd.Output()
	reply := strings.TrimSpace(string(out))
	if err != nil && reply == "" {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
			if len(msg) > 2000 {
				msg = msg[:2000]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "flowcli gagal: " + msg})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"reply": reply, "channel": "ratu-cli"})
}

func init() {
	RegisterFeature(Feature{Name: "cli-chat", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/cli-chat", cliChatHandler)
	}})
}
