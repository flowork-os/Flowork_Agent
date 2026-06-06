// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Resolver folder agent source-aware (fix bug.md #1) — cek source
//   repo (ProjectRoot/agents/<id>) dulu, baru fallback staged. Handler config/
//   toggle dulu cuma cek staged → source-agent ke-tolak "not found".

package agentmgr

import (
	"os"
	"path/filepath"

	"flowork-gui/internal/agentdb"
)

// agentSourceDir — folder source agent di repo (ProjectRoot/agents/<id>),
// "" kalau bukan source agent.
func agentSourceDir(id string) string {
	src := filepath.Join(agentdb.ProjectRoot(), "agents", id)
	if st, err := os.Stat(src); err == nil && st.IsDir() {
		return src
	}
	return ""
}

// resolveAgentDir — source folder dulu (authoritative), else staged. ok=false
// kalau dua-duanya ngga ada. Dir yang dibalikin aman buat agentdb.Resolve().
func resolveAgentDir(id string) (string, bool) {
	if src := agentSourceDir(id); src != "" {
		return src, true
	}
	staged := agentFolder(id)
	if st, err := os.Stat(staged); err == nil && st.IsDir() {
		return staged, true
	}
	return "", false
}
