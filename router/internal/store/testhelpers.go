// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import "sync"

func resetDBSingletonForTest() {
	if db != nil {
		_ = db.Close()
	}
	db = nil
	dbErr = nil
	dbOnce = sync.Once{}
}
