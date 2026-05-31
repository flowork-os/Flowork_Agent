// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Resolver root project tunggal (fix bug.md #2) — env override
//   FLOWORK_PROJECT_ROOT > os.Getwd(). Biar path source-agent ngga rapuh
//   pas binary dijalankan dari cwd lain. Dipakai Resolve/SourceWorkspace +
//   agentmgr + kernelhost.

package agentdb

import (
	"os"
	"strings"
)

// ProjectRoot — root source repo Flowork. Prioritas:
//   env FLOWORK_PROJECT_ROOT (explicit, anti-rapuh) > working directory.
// Jadi sumber kebenaran tunggal buat lokasi `agents/<id>/` + `workspace/`.
func ProjectRoot() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_PROJECT_ROOT")); v != "" {
		return v
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}
