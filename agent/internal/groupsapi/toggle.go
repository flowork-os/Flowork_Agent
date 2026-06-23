// 🔒 FROZEN GROUP-CORE · Repo: https://github.com/flowork-os/Flowork-OS · Owner: Aola Sahidin (Mr.Dev)
// ⛔ WAJIB sebelum ngedit file ini: BACA /home/mrflow/Documents/FLowork_os/lock/group.md
//    (cara kerja group, filtur, cara bikin group, CABANG *_ext.go). File ini BEKU (chattr +i +
//    hash KERNEL_FREEZE.md). Filtur baru → masuk *_ext.go (RegisterExecStrategy /
//    RegisterGroupSyncHook) atau DATA (Category/Directive). JANGAN buka file beku ini.
// ⚠️ EDITING A GROUP? READ doc/handbook/menu-group.md FIRST — the group list is auto-derived (any module with kv group=1); slash menu, ask_group, schedules and Mr.Flow all read the SAME synced list; on/off cascades to all members; reset restores from the repo. Do NOT hardcode the roster or kv "groups".
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-12
// Reason: Group ON/OFF — a single switch that disables/enables a group's
//
//	coordinator AND every member at once, so when a group is OFF no agent inside it
//	can ever receive a command (the coordinator can't fan out; members are unloaded).
//
// toggle.go — group-level on/off (cascade to members).
package groupsapi

import (
	"net/http"
	"strings"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/loket"
)

// ToggleHandler — POST /api/groups/toggle?id=<group>&disabled=1|0. Turns the whole
// group on/off in one shot: the coordinator and every roster member. OFF unloads
// them all, so nothing in the group runs or receives a command; ON brings them back.
func (h *Handler) ToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !idRe.MatchString(id) {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.d.Toggle == nil {
		writeErr(w, http.StatusInternalServerError, "toggle not wired")
		return
	}
	df := strings.TrimSpace(r.URL.Query().Get("disabled"))
	disabled := df == "1" || strings.EqualFold(df, "true")

	// Confirm it's a GROUP, collect its members, and record the on/off state in the
	// loket store (kv "group_off") so the GUI list can render the switch — the actual
	// gating is the per-agent disable below; this marker is just for display.
	targets := []string{id}
	isGroup := false
	if path, err := h.d.LoketStorePath(id); err == nil {
		if st, serr := loket.OpenStore(path); serr == nil {
			if marker, _, _ := st.KVGet("group"); strings.TrimSpace(marker) == "1" {
				isGroup = true
			}
			members, _, _ := st.KVGet("members")
			for _, m := range splitCSV(members) {
				if m != "" && m != id {
					targets = append(targets, m)
				}
			}
			if isGroup {
				off := "0"
				if disabled {
					off = "1"
				}
				_ = st.KVSet("group_off", off)
			}
			_ = st.Close()
		}
	}
	if !isGroup {
		writeErr(w, http.StatusForbidden, "bukan group — pakai /api/agents/toggle untuk agent biasa")
		return
	}

	applied, failed := []string{}, map[string]string{}
	for _, t := range targets {
		if err := h.d.Toggle(t, disabled); err == nil {
			applied = append(applied, t)
		} else {
			failed[t] = err.Error()
		}
	}
	// Re-sync the orchestrator so an OFF group drops from the Telegram slash menu +
	// Mr.Flow's list immediately (and an ON group reappears).
	h.SyncToOrchestrator()
	httpx.WriteJSON(w, map[string]any{
		"ok": true, "id": id, "disabled": disabled, "applied": applied, "failed": failed,
	})
}
