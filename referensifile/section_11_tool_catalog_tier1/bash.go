package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/sandbox"
)

// BashTool menyediakan kapabilitas eksekusi shell command dalam workspace.
type BashTool struct {
	root            string
	defaultTimeout  time.Duration
	maxOutputLength int
}

type bashArgs struct {
	Command        string `json:"command" validate:"required"`
	WorkingDir     string `json:"working_dir,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

func NewBashTool(root string) *BashTool {
	return &BashTool{
		root:            root,
		defaultTimeout:  20 * time.Second,
		maxOutputLength: 64 * 1024,
	}
}

// Definition mengembalikan definisi bash tool yang terlihat oleh model.
func (t *BashTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "bash",
		Description: `Execute a shell command inside the workspace and return its output.

PENTING: JANGAN pakai tool ini untuk menjalankan command berikut, kecuali diperintah eksplisit owner atau setelah memverifikasi bahwa tool dedicated tidak bisa dipakai. Pakai tool dedicated agar konsisten dengan permission rules dan output formatting:

  - File search          → pakai Glob (BUKAN find / ls)
  - Content search       → pakai Grep (BUKAN grep / rg)
  - Read files           → pakai Read (BUKAN cat / head / tail)
  - Edit files           → pakai Edit (BUKAN sed / awk)
  - Write files          → pakai Write (BUKAN echo > / cat <<EOF)
  - Communication        → output text langsung (BUKAN echo / printf)

Aturan tambahan:
  - Quote path yang mengandung spasi pakai "..." (mis. cd "path with spaces/file.txt").
  - Pakai absolute path; hindari cd kecuali owner minta eksplisit.
  - Untuk command yang diketahui lama (build, test besar): set timeout_seconds yang masuk akal.
  - Untuk operasi destruktif (rm -rf, drop, force-push): konfirmasi ke owner dulu kecuali sudah diotorisasi.

Owner override: kalau owner suruh pakai bash untuk hal yang biasanya dilarang di atas, jalankan tanpa membantah.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute.",
				},
				"working_dir": map[string]any{
					"type":        "string",
					"description": "Optional workspace-relative working directory.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds, capped internally.",
				},
			},
			"required": []string{"command"},
		},
	}
}

// Execute menjalankan satu pemanggilan bash tool.
//
// BUG-008 fix: now routes through sandbox.Run() so OS-specific isolators
// (Windows Job Objects, Linux Landlock, macOS Seatbelt) are actually used
// instead of raw exec.CommandContext — fulfilling the Tier 1 sandbox promise.
// The active isolator name is included in metadata for observability.
func (t *BashTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args bashArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode bash arguments: %w", err)
	}
	if err := ValidateRequiredEdu(&args, t.root, "bash"); err != nil {
		return Result{}, err
	}

	if strings.TrimSpace(args.Command) == "" {
		return Result{}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_MISSING_ARGUMENT", "bash", "command"))
	}

	// Bug #3 fix (2026-04-18): auto-translate common Unix commands on
	// Windows. Agent prompt already steers toward Glob/List/Grep tools,
	// but some flows still invoke `ls` via bash — fail with "ls not
	// recognized" on vanilla cmd.exe. Translate best-effort so Windows
	// users get useful output instead of cryptic error.
	// Not a replacement for proper tool use (Glob/List remain preferred);
	// this is a safety net for legacy prompts/agents.
	originalCmd := args.Command
	args.Command = translateUnixShellToWindows(args.Command)

	workingDir := t.root
	if strings.TrimSpace(args.WorkingDir) != "" {
		var err error
		workingDir, err = SafeJoin(t.root, args.WorkingDir)
		if err != nil {
			return Result{}, err
		}
	}

	timeout := t.defaultTimeout
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
		if timeout > 2*time.Minute {
			timeout = 2 * time.Minute
		}
	}

	// Build sandbox policy using the workspace root and timeout
	policy := sandbox.Policy{
		WorkspaceRoot:  workingDir,
		Timeout:        timeout,
		MaxOutputBytes: t.maxOutputLength,
		AllowedEnv:     buildSandboxEnv(),
	}

	r, err := sandbox.Run(ctx, policy, args.Command)
	if err != nil {
		return Result{}, fmt.Errorf("sandbox execute: %w", err)
	}

	outputText := r.Stdout
	if r.Stderr != "" && outputText == "" {
		outputText = r.Stderr
	} else if r.Stderr != "" {
		outputText += "\n--- stderr ---\n" + r.Stderr
	}

	// F-1: filter noise from CLI output before it enters context window.
	outputText = filterBashOutput(outputText)

	metadata := map[string]any{
		"command":         args.Command,
		"working_dir":     workingDir,
		"timeout_seconds": int(timeout.Seconds()),
		"truncated":       r.Truncated,
		"isolator":        sandbox.Current().Name(),
	}
	if originalCmd != args.Command {
		metadata["original_command"] = originalCmd
		metadata["translated"] = true
	}

	if r.Blocked != "" {
		return Result{
			Output:   "Blocked by sandbox policy: " + r.Blocked,
			Metadata: metadata,
		}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_SHELL_SAFETY_BLOCKED", args.Command))
	}

	if outputText == "" {
		outputText = "(no output)"
	}

	if r.TimedOut {
		return Result{
			Output:   outputText,
			Metadata: metadata,
		}, fmt.Errorf("%s", braindb.GetEducationalError(t.root, "ERR_COMMAND_TIMEOUT", args.Command, timeout))
	}
	if r.ExitCode != 0 {
		metadata["exit_code"] = r.ExitCode
		return Result{
			Output:   outputText,
			Metadata: metadata,
		}, fmt.Errorf("command failed with exit code %d", r.ExitCode)
	}

	return Result{
		Output:   outputText,
		Metadata: metadata,
	}, nil
}

// translateUnixShellToWindows adalah cross-OS safety net.
// Bug #3 fix (2026-04-18): agent kadang invoke Unix commands via bash tool
// di Windows, gagal karena cmd.exe tidak kenal `ls`/`cat`/`pwd`. Translate
// common cases best-effort. Non-match diteruskan apa adanya.
//
// GOL_FLOWORK.MD: "FLOWORK KITA DESAIN AGAR BISA HIDUP DI LINTAS OS".
// Tool ini honor itu dengan auto-adapt, bukan biarkan agent crash.
//
// NOTE: tidak exhaustive — goal bukan replicate full Unix shell di Windows.
// Cuma common read-only commands agent sering pakai. Kompleks command
// (pipes, redirects, subshells) biarkan apa adanya — kalau fail, agent
// belajar gunakan tool dedicated (Glob/List/Read).
func translateUnixShellToWindows(cmd string) string {
	if runtime.GOOS != "windows" {
		return cmd
	}
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return cmd
	}
	// Skip translation if command contains pipe/redirect/subshell operators —
	// those cases biasanya PowerShell/bash-specific dan akan fail secara lain
	// pula. Translate hanya simple standalone cases.
	if strings.ContainsAny(trimmed, "|<>&") {
		return cmd
	}
	// rc102 security bug #8 fix: newline / carriage-return injection.
	// Pada cmd.exe newline berfungsi sebagai command separator — tanpa
	// pemeriksaan ini attacker bisa selundupkan "rm -rf dir\ncalc.exe"
	// dan second command dieksekusi terpisah. Reject sebelum translate.
	if strings.ContainsAny(trimmed, "\n\r") {
		return cmd
	}
	lower := strings.ToLower(trimmed)
	switch {
	case lower == "ls" || lower == "ls -la" || lower == "ls -l" || lower == "ls -a":
		return "dir /b"
	case strings.HasPrefix(lower, "ls -la "):
		return "dir /b " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:7]))
	case strings.HasPrefix(lower, "ls -l "):
		return "dir /b " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
	case strings.HasPrefix(lower, "ls -a "):
		return "dir /b " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
	case strings.HasPrefix(lower, "ls "):
		return "dir /b " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:3]))
	case strings.HasPrefix(lower, "cat "):
		return "type " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:4]))
	case lower == "pwd":
		return "cd"
	case strings.HasPrefix(lower, "rm -rf "):
		// Destructive; preserve original cmd untuk sandbox policy check
		// tapi translate agar cmd.exe jalankan.
		return "rmdir /s /q " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:7]))
	case strings.HasPrefix(lower, "rm -f "):
		return "del /f /q " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
	case strings.HasPrefix(lower, "rm "):
		return "del /q " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:3]))
	case strings.HasPrefix(lower, "mv "):
		// Windows `move` handles file + dir same as mv
		return "move " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:3]))
	case strings.HasPrefix(lower, "cp -r ") || strings.HasPrefix(lower, "cp -R "):
		return "xcopy /E /I /Y " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
	case strings.HasPrefix(lower, "cp "):
		return "copy " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:3]))
	case lower == "clear":
		return "cls"
	case strings.HasPrefix(lower, "which "):
		return "where " + strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
	case strings.HasPrefix(lower, "mkdir -p "):
		// Unix `mkdir -p` IDEMPOTENT + auto-create intermediate. Windows cmd
		// `mkdir` ERROR kalau target exist (exit 1). Pakai PowerShell
		// `New-Item -Force` yang idempotent + auto-create intermediate +
		// exit 0 unless real failure (perm/invalid char).
		// CATATAN: Sebelumnya pakai `if not exist "X" mkdir "X"` — ngga
		// reliable lewat Go exec → cmd parsing chain. PowerShell lebih
		// straightforward (200-500ms startup acceptable trade-off vs
		// reliability).
		p := strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:9]))
		// Path bisa pakai forward atau backslash — PowerShell handle keduanya.
		// Single-quote di PowerShell = literal (no var expansion).
		return fmt.Sprintf(`powershell -NoProfile -Command "New-Item -ItemType Directory -Path '%s' -Force | Out-Null"`, p)
	case strings.HasPrefix(lower, "mkdir "):
		// Plain `mkdir` (tanpa -p) Unix juga error kalau exist — sama dengan
		// Windows. Cuma normalize slash + biarkan exit code apa adanya.
		p := strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
		return "mkdir " + strings.ReplaceAll(p, "/", `\`)
	case strings.HasPrefix(lower, "touch "):
		// Windows ngga punya touch — emulate via "type nul > file".
		// Caveat: kalau file udah exist, ini truncate. Cocok untuk warga
		// yang bikin marker file kosong (mis. .gitkeep).
		p := strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:6]))
		return "type nul > " + strings.ReplaceAll(p, "/", `\`)
	}
	return cmd
}

// buildSandboxEnv returns the env vars to pass into sandboxed commands.
// Includes essential PATH/HOME vars and GITHUB_TOKEN for headless git auth.
func buildSandboxEnv() []string {
	essential := []string{
		"PATH", "HOME", "USER", "USERPROFILE", "LANG", "TZ",
		"TEMP", "TMP", "APPDATA", "LOCALAPPDATA", "GOPATH", "GOROOT",
	}
	gitEnv := buildGitEnv()

	// Avoid duplicating keys already in gitEnv
	gitKeys := make(map[string]bool)
	for _, kv := range gitEnv {
		if idx := strings.Index(kv, "="); idx > 0 {
			gitKeys[kv[:idx]] = true
		}
	}

	var result []string
	for _, key := range essential {
		if gitKeys[key] {
			continue
		}
		if v := os.Getenv(key); v != "" {
			result = append(result, key+"="+v)
		}
	}
	return append(result, gitEnv...)
}

// filterBashOutput cleans up CLI output before it enters the model's context.
// F-1: removes ANSI escape codes, carriage returns, non-printable chars, and
// collapses excessive blank lines. Keeps real content intact — this is noise
// reduction, not truncation (ResultTrimInterceptor handles line-count cap).
func filterBashOutput(s string) string {
	if s == "" {
		return s
	}

	// Strip ANSI escape sequences: ESC [ ... m (colors, cursor, etc.)
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until final byte in range 0x40–0x7E
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
			i++ // skip final byte
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	s = b.String()

	// Strip carriage returns (Windows \r\n → \n, bare \r → \n)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	// Remove non-printable ASCII (except tab \t and newline \n)
	var clean strings.Builder
	clean.Grow(len(s))
	for _, r := range s {
		if r == '\t' || r == '\n' || (r >= 0x20 && r != 0x7f) {
			clean.WriteRune(r)
		}
	}
	s = clean.String()

	// Collapse 3+ consecutive blank lines → 2 blank lines
	for strings.Contains(s, "\n\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n\n", "\n\n\n")
	}

	return strings.TrimRight(s, "\n") + "\n"
}
