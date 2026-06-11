// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-12
// Reason: PLUG-AND-PLAY group discovery. SyncToOrchestrator auto-discovers every
//   group (kv group=1) and writes the "id|command|desc" list into the orchestrator's
//   kv "groups" — the single source mr-flow-next reads to build the Telegram slash
//   menu (groupCommandsJSON) and the ask_group tool. So a NEW group appears in the
//   bot's slash commands and becomes runnable via Mr.Flow automatically — no kv
//   editing, no code change per group. Called on boot + on create/config/delete.
//
// orchestrator.go — auto-sync the group list to the orchestrator (plug-and-play).
package groupsapi

import (
	"strings"

	"flowork-gui/internal/loket"
)

// OrchestratorID is the agent that owns the Telegram slash menu + the ask_group
// tool (the agent telegram-channel routes to). It reads its kv "groups" to know
// which groups exist; the host keeps that list in sync automatically.
const OrchestratorID = "mr-flow-next"

// sanitizeDesc strips the delimiters the kv "groups" format uses (";" and "|") and
// newlines so a group's free-text task/display can't corrupt the list.
func sanitizeDesc(s string) string {
	s = strings.NewReplacer(";", ",", "|", "/", "\n", " ", "\r", " ").Replace(strings.TrimSpace(s))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// SyncToOrchestrator auto-discovers every group (kv group=1) across all loaded
// modules and writes the "id|command|desc" list (";"-joined) into the orchestrator's
// kv "groups". Idempotent; safe to call often. Returns how many groups it synced.
// This is the plug-and-play core: create a group → it shows up everywhere, no config.
func (h *Handler) SyncToOrchestrator() int {
	if h.d.AgentIDs == nil || h.d.LoketStorePath == nil {
		return 0
	}
	var parts []string
	for _, id := range h.d.AgentIDs() {
		if id == OrchestratorID {
			continue
		}
		path, err := h.d.LoketStorePath(id)
		if err != nil {
			continue
		}
		st, err := loket.OpenStore(path)
		if err != nil {
			continue
		}
		marker, _, _ := st.KVGet("group")
		if strings.TrimSpace(marker) == "1" {
			desc := h.displayName(id)
			if dn, ok, _ := st.KVGet("display_name"); ok && strings.TrimSpace(dn) != "" {
				desc = strings.TrimSpace(dn)
			} else if task, _, _ := st.KVGet("task"); strings.TrimSpace(task) != "" {
				desc = strings.TrimSpace(task)
			}
			// command = the group id (already a safe lowercase slug, valid Telegram
			// command shape); desc = friendly text shown in the slash menu.
			parts = append(parts, id+"|"+id+"|"+sanitizeDesc(desc))
		}
		_ = st.Close()
	}

	opath, err := h.d.LoketStorePath(OrchestratorID)
	if err != nil {
		return 0
	}
	ost, err := loket.OpenStore(opath)
	if err != nil {
		return 0
	}
	_ = ost.KVSet("groups", strings.Join(parts, ";"))
	_ = ost.Close()
	return len(parts)
}
