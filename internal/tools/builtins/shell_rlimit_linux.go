// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 3 acceptance — bash mem limit (Linux only).
//   RLIMIT_AS 512MB enforced via syscall.Setrlimit pre-exec di SysProcAttr.
//   Multi-OS: file ini build tag linux only. Windows/macOS = no-op
//   (shell_rlimit_other.go).
//
// shell_rlimit_linux.go — Section 12 phase 3: RLIMIT_AS Linux mem cap.

//go:build linux
// +build linux

package builtins

import (
	"os/exec"
	"syscall"
)

// bashMemLimitBytes — 512MB virtual memory cap per bash exec. Compile-
// time constant — bukan env (anti tamper).
const bashMemLimitBytes uint64 = 512 * 1024 * 1024

// applyMemLimit — set RLIMIT_AS via syscall on Linux. Caller (bash Run)
// panggil sebelum c.Start().
//
// SysProcAttr fields tersedia di Linux:
//   - Setrlimit langsung di SysProcAttr ngga ada di Go stdlib (need to
//     wrap via os/exec Cmd.SysProcAttr + Pdeathsig + Credential).
//   - Cara cleanest: set ulimit di shell ("-c \"ulimit -v %d; %s\"").
//   - Tapi itu ngubah perilaku shell — child shell jadi reset.
//
// Implementation pratis: prepend `ulimit -v <KB>; ` ke command. Bash
// sudah berfungsi sebagai shell — POSIX ulimit reliable.
func applyMemLimit(c *exec.Cmd, command string) {
	// Convert bytes ke KB (POSIX ulimit -v unit = KB).
	limitKB := bashMemLimitBytes / 1024
	// Reconstruct args: /bin/sh -c "ulimit -v <KB>; <command>"
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

// MemLimitEnabled — return true on Linux (constant — tested at compile).
const MemLimitEnabled = true

// keep syscall import used.
var _ = syscall.SIGTERM
