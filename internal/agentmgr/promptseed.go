// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-12
// Reason: Make every agent's persona GUI-editable. SeedPromptsFromMd backfills the
//   GUI prompt (kv.prompt) from an agent's prompt.md when unset, so the persona
//   shows up and is editable in Settings → Prompt and reaches the wasm via
//   FLOWORK_AGENT_CONFIG.prompt. Idempotent + non-destructive (never overwrites an
//   owner-edited prompt). Wired into boot (main.go) before kernelhost.Boot.
//
// promptseed.go — backfill GUI-editable prompt from prompt.md.
package agentmgr

import (
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
)

// SeedPromptsFromMd backfills each agent's GUI-editable prompt (kv.prompt) from its
// prompt.md file when the prompt isn't set yet. Result: the persona appears and is
// editable in the GUI, and reaches the wasm via FLOWORK_AGENT_CONFIG.prompt — so a
// user repurposing Flowork edits the prompt in ONE place (the GUI), never a file.
// ONLY seeds when kv.prompt is empty, so it never clobbers a GUI-edited prompt.
// Returns how many it seeded. Call at boot, BEFORE kernelhost.Boot (so the seeded
// prompt is in the agent's env on first load).
func SeedPromptsFromMd(agentsDir string) int {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return 0
	}
	seeded := 0
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), ".fwagent") {
			continue
		}
		dir := filepath.Join(agentsDir, e.Name())
		raw, rerr := os.ReadFile(filepath.Join(dir, "prompt.md"))
		if rerr != nil {
			continue // no prompt.md to seed from
		}
		prompt := strings.TrimSpace(string(raw))
		if prompt == "" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".fwagent")
		st, serr := agentdb.Open(agentdb.Resolve(id, dir))
		if serr != nil {
			continue
		}
		if cur, _ := st.GetPrompt(); strings.TrimSpace(cur) == "" {
			if st.SetPrompt(prompt) == nil {
				seeded++
			}
		}
		_ = st.Close()
	}
	return seeded
}
