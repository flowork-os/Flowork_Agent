// 🔒 FROZEN GROUP-CORE · Repo: https://github.com/flowork-os/Flowork-OS · Owner: Aola Sahidin (Mr.Dev)
// ⛔ WAJIB sebelum ngedit file ini: BACA /home/mrflow/Documents/FLowork_os/lock/group.md
//    (cara kerja group, filtur, cara bikin group, CABANG *_ext.go). File ini BEKU (chattr +i +
//    hash KERNEL_FREEZE.md). Filtur baru → masuk *_ext.go (RegisterExecStrategy /
//    RegisterGroupSyncHook) atau DATA (Category/Directive). JANGAN buka file beku ini.
// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked at: 2026-06-15 (owner-approved autonomous sprint)
// Reason: CreateGroup — programmatic group deploy+roster for the Architect. Companion to
//
//	the soft-LOCKED groupsapi.go (left untouched). VERIFIED: created group "peramal" with
//	4 members + synthesizer; group.json + loket group=1 written; SyncToOrchestrator ran;
//	coordinator hot-loaded; chattable immediately. Mirrors CreateHandler+ConfigHandler.
//
// create_team.go — PROGRAMMATIC group creation for the Flowork Architect.
//
// groupsapi.go is a soft-LOCKED file; rather than touch it, this companion file
// (same package → can reach its unexported helpers idRe / groupManifest /
// writeGroupSeed / SyncToOrchestrator and the Handler's Deps) adds ONE method:
// CreateGroup — the programmatic equivalent of CreateHandler + ConfigHandler in a
// single call. The Architect generates the member agents first, then calls this to
// stand up the coordinator group that fans out to them. No restart needed: the
// hot-reload watcher loads the new <id>.fwagent, and SyncToOrchestrator registers
// its Telegram slash command + ask_group entry — exactly the GUI's plug-and-play
// path, just driven from code.
package groupsapi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/loket"
)

// CreateGroup deploys (or updates) a GROUP module and sets its roster atomically.
// If the coordinator folder doesn't exist yet it is created from the shared group
// wasm; if it already exists only the roster is refreshed (so re-building a team
// updates it instead of failing on a name clash). members/synthesizer must be ids
// of agents that already exist (the Architect installs them before calling this).
func (h *Handler) CreateGroup(id, displayName string, members []string, synthesizer, task string) error {
	id = strings.ToLower(strings.TrimSpace(id))
	if !idRe.MatchString(id) {
		return fmt.Errorf("group id invalid (^[a-z0-9][a-z0-9-]{1,39}$): %q", id)
	}
	display := strings.TrimSpace(displayName)
	if display == "" {
		display = id
	}
	dir := filepath.Join(h.d.AgentsDir, id+".fwagent")

	// Deploy the coordinator module only if it isn't there yet — a re-build of an
	// existing team is a roster update, not a re-deploy.
	if _, statErr := os.Stat(dir); statErr != nil {
		wasm, werr := os.ReadFile(h.d.GroupWasmPath)
		if werr != nil {
			return fmt.Errorf("group template wasm not found (%s): %w", h.d.GroupWasmPath, werr)
		}
		if err := os.MkdirAll(filepath.Join(dir, "workspace"), 0o755); err != nil {
			return fmt.Errorf("mkdir group: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), groupManifest(id, display), 0o644); err != nil {
			_ = os.RemoveAll(dir)
			return fmt.Errorf("write manifest: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "agent.wasm"), wasm, 0o644); err != nil {
			_ = os.RemoveAll(dir)
			return fmt.Errorf("write wasm: %w", err)
		}
		_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("agent.wasm\nworkspace/*.db\nworkspace/*.db-*\n"), 0o644)
	}

	// Roster → the group's own loket store (read live by the coordinator wasm).
	clean := []string{}
	for _, m := range members {
		if m = strings.TrimSpace(m); m != "" && m != id {
			clean = append(clean, m)
		}
	}
	path, perr := h.d.LoketStorePath(id)
	if perr != nil {
		return fmt.Errorf("loket path: %w", perr)
	}
	st, serr := loket.OpenStore(path)
	if serr != nil {
		return fmt.Errorf("open loket store: %w", serr)
	}
	// Chain every KVSet so any failure surfaces (same discipline as ConfigHandler —
	// a swallowed first error used to report success while nothing persisted).
	err := st.KVSet("group", "1")
	if err == nil {
		err = st.KVSet("members", strings.Join(clean, ","))
	}
	if err == nil {
		err = st.KVSet("synthesizer", strings.TrimSpace(synthesizer))
	}
	if err == nil {
		err = st.KVSet("task", strings.TrimSpace(task))
	}
	if err == nil {
		err = st.KVSet("display_name", display)
	}
	_ = st.Close()
	if err != nil {
		return fmt.Errorf("write roster: %w", err)
	}

	// Mirror to a secret-free group.json (portable/committable) + register with the
	// orchestrator (Telegram slash menu + ask_group) — plug-and-play, no restart.
	writeGroupSeed(h.d.AgentsDir, id, clean, synthesizer, task, display)
	h.SyncToOrchestrator()
	return nil
}
