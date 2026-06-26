// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"errors"

	"flowork-gui/internal/agentdb"
)

func ToggleAgent(id string, disabled bool) error {
	if !reID.MatchString(id) {
		return errors.New("invalid id")
	}
	dir, ok := resolveAgentDir(id)
	if !ok {
		return errors.New("agent not found")
	}
	store, err := agentdb.Open(agentdb.Resolve(id, dir))
	if err != nil {
		return err
	}
	if err := store.SetDisabled(disabled); err != nil {
		store.Close()
		return err
	}
	store.Close()
	if Reload != nil {
		return Reload(id)
	}
	return nil
}
