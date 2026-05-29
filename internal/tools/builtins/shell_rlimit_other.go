// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 3 — non-Linux (Windows/macOS/Darwin) no-op
//   memory limiter. Future: macOS via launchd resource limits, Windows
//   via Job Object SetInformationJobObject. Phase 4+ → tambah file
//   build-tag.
//
// shell_rlimit_other.go — Section 12 phase 3: no-op stub.

//go:build !linux
// +build !linux

package builtins

import "os/exec"

func applyMemLimit(c *exec.Cmd, command string) {
	// no-op on non-Linux.
	_ = c
	_ = command
}

const MemLimitEnabled = false
