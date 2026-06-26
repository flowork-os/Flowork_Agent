// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

//go:build !windows

package updater

import (
	"os"
	"syscall"
)

func restartImpl(exe string) error {
	return syscall.Exec(exe, os.Args, os.Environ())
}
