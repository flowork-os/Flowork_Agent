// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-12
// Reason: Reset-to-default. If a group (or a member) is deleted by accident,
//   ResetHandler copies its bundled DEFINITION back from the repo (repo/agents) into
//   the runtime, restores its roster from group.json, and re-syncs — the agent
//   reappears with its factory setup. Never touches an agent that already exists, and
//   never copies workspace/state.db (no secrets, no clobbering live data).
//
// reset.go — restore bundled (factory) groups/agents.
package groupsapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

// ResetHandler — POST /api/groups/reset. Restores every bundled agent
// (repo/agents/<id>.fwagent) that's missing from the runtime: copies its definition
// (manifest, wasm, loket.json, group.json, prompt.md, …) — never workspace/*.db —
// then restores rosters from group.json and re-syncs the orchestrator. The hot-reload
// watcher loads the restored agents. Existing agents are left untouched.
func (h *Handler) ResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	repo := filepath.Join(agentdb.ProjectRoot(), "agents")
	entries, err := os.ReadDir(repo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "repo agents: "+err.Error())
		return
	}
	restored := []string{}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".fwagent") {
			continue
		}
		dst := filepath.Join(h.d.AgentsDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue // already installed — never clobber live data
		}
		if err := copyAgentDef(filepath.Join(repo, e.Name()), dst); err == nil {
			restored = append(restored, strings.TrimSuffix(e.Name(), ".fwagent"))
		}
	}
	h.SeedFromJSON()       // restore rosters from the committed group.json mirrors
	h.SyncToOrchestrator() // re-register slash + ask_group for the restored groups
	httpx.WriteJSON(w, map[string]any{"ok": true, "restored": restored, "count": len(restored)})
}

// copyAgentDef copies an agent's DEFINITION files (top-level only — never the
// workspace/ runtime dir or any *.db) from src to dst.
func copyAgentDef(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue // skip workspace/ and shared/ (runtime/secret)
		}
		if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-shm") || strings.HasSuffix(name, ".db-wal") {
			continue
		}
		if err := copyFile(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
