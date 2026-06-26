// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package tools

import "strings"

var surfaceToCanonical = map[string]string{
	"read":            "file_read",
	"write":           "file_write",
	"edit":            "edit",
	"bash":            "bash",
	"grep":            "grep",
	"glob":            "glob",
	"websearch":       "web_search",
	"webfetch":        "webfetch",
	"toolsearch":      "tool_search",
	"skill":           "skill",
	"agent":           "agent_command",
	"askuserquestion": "askuser",
}

var canonToSurface = map[string]string{
	"file_read":     "Read",
	"file_write":    "Write",
	"edit":          "Edit",
	"bash":          "Bash",
	"grep":          "Grep",
	"glob":          "Glob",
	"web_search":    "WebSearch",
	"webfetch":      "WebFetch",
	"tool_search":   "ToolSearch",
	"skill":         "Skill",
	"agent_command": "Agent",
	"askuser":       "AskUserQuestion",
}

func canonicalToolName(name string) string {
	n := strings.TrimSpace(name)
	if c, ok := surfaceToCanonical[strings.ToLower(n)]; ok {
		return c
	}
	return n
}

func DisplayName(canonical string) string {
	if d, ok := canonToSurface[canonical]; ok {
		return d
	}
	return canonical
}
