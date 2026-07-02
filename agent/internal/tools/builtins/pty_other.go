//go:build !linux

// pty_other.go — SIBLING ext (deletable, NON-frozen): stub PTY buat OS non-Linux.
// exec interaktif via /dev/ptmx cuma di Linux; OS lain balikin error jelas (tool
// tetep ke-register kalau FLOWORK_PTY=1, tapi start-nya gagal sopan). 📄 lock/pty-exec.md
package builtins

import "fmt"

func startPTYSession(_, _, _ string) (*ptySession, error) {
	return nil, fmt.Errorf("pty exec cuma didukung di Linux (OS ini belum) — pakai tool `shell`/`bash` buat non-interaktif")
}
