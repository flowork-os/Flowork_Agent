// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
)

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
