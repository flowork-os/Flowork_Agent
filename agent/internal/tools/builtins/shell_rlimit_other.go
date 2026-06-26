// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

//go:build !linux
// +build !linux

package builtins

import "os/exec"

func applyMemLimit(c *exec.Cmd, command string) {

	_ = c
	_ = command
}

const MemLimitEnabled = false
