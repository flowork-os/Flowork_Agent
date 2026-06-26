// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

//go:build linux
// +build linux

package builtins

import (
	"os/exec"
	"syscall"
)

const bashMemLimitBytes uint64 = 512 * 1024 * 1024

func applyMemLimit(c *exec.Cmd, command string) {

	limitKB := bashMemLimitBytes / 1024

	if len(c.Args) >= 3 && c.Args[0] == "/bin/sh" && c.Args[1] == "-c" {
		c.Args[2] = wrapULimit(limitKB, command)
	}
}

func wrapULimit(kb uint64, command string) string {
	return "ulimit -v " + uintToStr(kb) + " 2>/dev/null; " + command
}

func uintToStr(n uint64) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

const MemLimitEnabled = true

var _ = syscall.SIGTERM
