// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-12
// Reason: Portable GROUP roster mirror. ReconcileGroupSeeds (SeedFromJSON) syncs a
//   group's roster between its loket store and a secret-free group.json both ways;
//   never creates a store for a non-group module. Audited + build/test green.
//
// seed.go — portable GROUP roster mirror (group.json).
//
// A group's roster (members/synthesizer/task) lives in its loket store, which is
// a .db file (git-ignored, may hold runtime state). To let a group travel with
// the repo, the roster is mirrored to a secret-free group.json in the group's
// folder (tracked). ReconcileGroupSeeds keeps the two in sync on boot, in BOTH
// directions:
//   - export: a configured group on this machine (loket has a roster) whose
//     group.json is missing/stale gets it (re)written — so it can be committed.
//   - import: a fresh install (group.json present, loket store empty) gets the
//     roster restored into the loket store — so the colony works with no re-config.
package groupsapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/loket"
)

// groupSeed is the portable, secret-free roster snapshot stored as group.json.
type groupSeed struct {
	Members     []string `json:"members"`
	Synthesizer string   `json:"synthesizer"`
	Task        string   `json:"task"`
	DisplayName string   `json:"display_name,omitempty"`
}

func groupJSONPath(agentsDir, id string) string {
	return filepath.Join(agentsDir, id+".fwagent", "group.json")
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// writeGroupSeed mirrors a roster to group.json. Best-effort — a failure here
// never blocks the caller (the loket store is the runtime source of truth).
func writeGroupSeed(agentsDir, id string, members []string, synth, task, display string) {
	s := groupSeed{Members: members, Synthesizer: strings.TrimSpace(synth), Task: strings.TrimSpace(task), DisplayName: strings.TrimSpace(display)}
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(groupJSONPath(agentsDir, id), append(b, '\n'), 0o644)
	}
}

func splitMembers(csv string) []string {
	out := []string{}
	for _, m := range strings.Split(csv, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	return out
}

// SeedFromJSON reconciles every group's roster between its loket store and its
// group.json mirror (see package doc). Returns (restored, exported) counts. Safe
// to call on every boot; it never creates a loket store for a non-group module.
func (h *Handler) SeedFromJSON() (restored, exported int) {
	if h.d.LoketStorePath == nil {
		return 0, 0
	}
	entries, err := os.ReadDir(h.d.AgentsDir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".fwagent") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".fwagent")
		jsonPath := groupJSONPath(h.d.AgentsDir, id)
		loketPath, perr := h.d.LoketStorePath(id)
		if perr != nil {
			continue
		}
		hasJSON := fileExists(jsonPath)
		hasLoket := fileExists(loketPath)
		// Only touch modules that are (or claim to be) groups; never create a
		// loket store for a plain agent that has neither a store nor a mirror.
		if !hasJSON && !hasLoket {
			continue
		}

		st, serr := loket.OpenStore(loketPath)
		if serr != nil {
			continue
		}
		marker, _, _ := st.KVGet("group")
		curMembers, _, _ := st.KVGet("members")

		switch {
		case strings.TrimSpace(curMembers) != "":
			// Loket has a roster → export to group.json (write if missing/stale).
			synth, _, _ := st.KVGet("synthesizer")
			task, _, _ := st.KVGet("task")
			disp, _, _ := st.KVGet("display_name")
			next := groupSeed{Members: splitMembers(curMembers), Synthesizer: synth, Task: task, DisplayName: disp}
			if !hasJSON || !sameSeed(jsonPath, next) {
				writeGroupSeed(h.d.AgentsDir, id, next.Members, synth, task, disp)
				exported++
			}
		case hasJSON:
			// Empty loket but a committed mirror → import (fresh-install restore).
			raw, rerr := os.ReadFile(jsonPath)
			if rerr == nil {
				var s groupSeed
				if json.Unmarshal(raw, &s) == nil && (len(s.Members) > 0 || strings.TrimSpace(s.Task) != "") {
					_ = st.KVSet("group", "1")
					_ = st.KVSet("members", strings.Join(s.Members, ","))
					_ = st.KVSet("synthesizer", strings.TrimSpace(s.Synthesizer))
					_ = st.KVSet("task", strings.TrimSpace(s.Task))
					if strings.TrimSpace(s.DisplayName) != "" {
						_ = st.KVSet("display_name", strings.TrimSpace(s.DisplayName))
					}
					restored++
				}
			}
		}
		_ = marker
		_ = st.Close()
	}
	return restored, exported
}

// sameSeed reports whether group.json at path already holds exactly want.
func sameSeed(path string, want groupSeed) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var got groupSeed
	if json.Unmarshal(raw, &got) != nil {
		return false
	}
	if strings.TrimSpace(got.Synthesizer) != strings.TrimSpace(want.Synthesizer) ||
		strings.TrimSpace(got.Task) != strings.TrimSpace(want.Task) ||
		strings.TrimSpace(got.DisplayName) != strings.TrimSpace(want.DisplayName) ||
		len(got.Members) != len(want.Members) {
		return false
	}
	for i := range got.Members {
		if got.Members[i] != want.Members[i] {
			return false
		}
	}
	return true
}
