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
	"encoding/json"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
)

// starterPrompt returns the best available starter persona for an agent so the GUI
// Prompt field is never empty: the agent's prompt.md if present, otherwise a sentence
// built from its manifest ("You are <display_name>. <description>"). Empty only when
// the folder has neither — nothing to seed.
func starterPrompt(dir, id string) string {
	if raw, err := os.ReadFile(filepath.Join(dir, "prompt.md")); err == nil {
		if p := strings.TrimSpace(string(raw)); p != "" {
			return p
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return ""
	}
	var m struct {
		DisplayName string `json:"display_name"`
		Description string `json:"description"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = id
	}
	desc := strings.TrimSpace(m.Description)
	if desc == "" {
		return "You are " + name + ", an agent in Flowork. Be concise, honest, and never hallucinate."
	}
	return "You are " + name + ". " + desc + "\n\nBe concise, honest, and never claim a capability you don't have."
}

// SeedPromptsFromMd backfills each agent's GUI-editable prompt (kv.prompt) when it
// isn't set yet, so the Prompt field in the GUI is never empty: from the agent's
// prompt.md if present, otherwise a starter persona derived from its manifest. The
// prompt reaches the wasm via FLOWORK_AGENT_CONFIG.prompt, so a user repurposing
// Flowork edits it in ONE place (the GUI). ONLY seeds when kv.prompt is empty, so it
// never clobbers a GUI-edited prompt. Returns how many it seeded. Call at boot,
// BEFORE kernelhost.Boot (so the seeded prompt is in the agent's env on first load).
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
		prompt := starterPrompt(dir, strings.TrimSuffix(e.Name(), ".fwagent"))
		if prompt == "" {
			continue // no prompt.md and no manifest text to seed from
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
