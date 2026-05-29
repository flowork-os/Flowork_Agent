package tools

// interceptors_shell.go — ShellSafetyInterceptor.
// Mencegat fragment shell command yang jelas berbahaya (rm -rf /, mkfs,
// fork-bomb, dll) sebelum bash tool dieksekusi.
//
// Root opsional — kalau di-set, pesan error ditarik dari educational_errors
// (ERR_SHELL_SAFETY_BLOCKED). Karma -5 per breach.

import (
	"context"
	"fmt"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
)

// ShellSafetyInterceptor mencegat fragment shell command yang jelas berbahaya.
//
// Root opsional — kalau di-set, pesan error ditarik dari educational_errors
// (ERR_SHELL_SAFETY_BLOCKED). Kalau kosong (e.g. test), fallback generic.
type ShellSafetyInterceptor struct {
	Root string
}

// Before memeriksa sebelum eksekusi bash apakah command mengandung fragment berbahaya.
func (i ShellSafetyInterceptor) Before(_ context.Context, invocation *Invocation) error {
	if invocation.ToolName != "bash" {
		return nil
	}

	command, _ := stringArg(invocation.ParsedArgs, "command")
	command = strings.ToLower(command)
	disallowed := []string{
		"rm -rf /",
		"mkfs",
		"shutdown",
		"reboot",
		"poweroff",
		":(){:|:&};:",
	}

	for _, fragment := range disallowed {
		if strings.Contains(command, fragment) {
			_, _ = braindb.AdjustKarma(i.Root, currentAgent(), -5, "shell_safety_blocked: "+fragment)
			edu := braindb.GetEducationalError(i.Root, "ERR_SHELL_SAFETY_BLOCKED", fragment)
			return fmt.Errorf("%s\n\n[teknis: fragment=%q, karma -5]", edu, fragment)
		}
	}
	return nil
}

// After adalah post-hook shell safety interceptor; saat ini tidak melakukan apa pun.
func (ShellSafetyInterceptor) After(_ context.Context, _ Invocation, _ *Result, _ error) {}
