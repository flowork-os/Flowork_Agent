// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"os"
	"path/filepath"

	"flowork-gui/internal/agentdb"
)

func agentSourceDir(id string) string {
	src := filepath.Join(agentdb.ProjectRoot(), "agents", id)
	if st, err := os.Stat(src); err == nil && st.IsDir() {
		return src
	}
	return ""
}

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
