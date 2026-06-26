// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Profil cloaking via switch (FLOWORK_CLOAK_*) → dok lock/plug-and-play.md  ⚠️ FROZEN.
// Atur dari GUI fwswitch, bukan edit kode. Mekanisme cloak = cloaking.go (frozen). Cara:
// CARAFREEZE.MD (POLA B). Pola freeze: lock/frozen-core.md

package router

import (
	"os"
	"strings"
)

func cloakEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func claudeToolSuffix() string { return cloakEnv("FLOWORK_CLOAK_SUFFIX", "_cc") }
func claudeVersion() string    { return cloakEnv("FLOWORK_CLOAK_VERSION", "2.1.92") }

var defaultCCDecoyToolNames = []string{
	"Task", "TaskOutput", "TaskStop", "TaskCreate", "TaskGet", "TaskUpdate",
	"TaskList", "Bash", "Glob", "Grep", "Read", "Edit", "Write", "NotebookEdit",
	"WebFetch", "WebSearch", "AskUserQuestion", "Skill", "EnterPlanMode", "ExitPlanMode",
}

func ccDecoyToolNames() []string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_CLOAK_DECOYS")); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return defaultCCDecoyToolNames
}
