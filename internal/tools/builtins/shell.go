// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 11 phase 1c shell tool (bash) — adapted minimal from
//   referensifile/section_11/bash.go. Multi-OS: Linux/macOS via /bin/sh,
//   Windows via cmd.exe. Sandbox layer = Section 12 (cap gate + rate
//   limit). DENYLIST + timeout + output cap di sini. Phase 1c+ extension
//   (richer sandbox via Landlock/Job Object, exec_sandbox tool) →
//   tambah file baru, JANGAN modify ini.
//
// shell.go — Section 11 phase 1c: bash tool.
//
// Security model:
//   1. Capability gate (Section 12 sandbox) — agent harus punya `exec:shell`
//      di capabilities_required. Tanpa cap → denial sebelum Run dipanggil.
//   2. Pre-execution denylist: pattern berbahaya (fork bomb, rm -rf /,
//      chmod 777, sudo, curl|sh, eval $) — reject.
//   3. Working dir: dari ctx FromSharedDir (default agent shared workspace).
//      Anti escape — JANGAN accept `working_dir: ../` atau absolute path
//      di luar shared dir.
//   4. Timeout: default 20s, cap 60s (via ctx.WithTimeout).
//   5. Output cap: stdout+stderr combined 64KB. Sisanya truncated dengan
//      marker `[...truncated]`.
//   6. ExitCode = 0 sukses; non-zero non-error (caller liat exit_code).

package builtins

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"flowork-gui/internal/tools"
)

// bashTool — execute single shell command via sh / cmd.
type bashTool struct{}

func (bashTool) Name() string       { return "bash" }
func (bashTool) Capability() string { return "exec:shell" }
func (bashTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Execute shell command inside agent shared workspace. Linux/macOS via /bin/sh -c, Windows via cmd /C. Default timeout 20s (cap 60). Output cap 64KB. Denied patterns: rm -rf /, fork bomb, sudo, chmod 777, curl|sh, eval $(...).",
		Params: []tools.Param{
			{Name: "command", Type: tools.ParamString, Description: "shell command to execute", Required: true},
			{Name: "working_dir", Type: tools.ParamString, Description: "optional subdir inside shared workspace (default workspace root)"},
			{Name: "timeout_seconds", Type: tools.ParamInt, Description: "optional timeout 1..60 (default 20)"},
		},
		Returns: "{stdout, stderr, exit_code, truncated, duration_ms}",
	}
}

// shellDenyPatterns — substring match. Conservative — lebih baik false
// positive (block legit usecase yang jarang) daripada kelolosan attack.
var shellDenyPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"rm -rf ~",
	"rm --no-preserve-root",
	":(){:|:&};:", // classic fork bomb
	":() { :|: & };:",
	"sudo ",
	"su -",
	"chmod 777",
	"chown -R",
	"mkfs",
	"dd if=/dev/zero",
	"dd if=/dev/random",
	"> /dev/sda",
	"> /dev/nvme",
	"shutdown",
	"reboot",
	"halt",
	"poweroff",
	"init 0",
	"init 6",
	"|sh",
	"| sh",
	"|bash",
	"| bash",
	"curl -s http", // generic curl|sh pattern — caller pakai webfetch tool
	"wget -O -",
	"eval $",
	"eval `",
	"; :(){",
	"/etc/passwd",
	"/etc/shadow",
	"~/.ssh/",
	".ssh/id_rsa",
}

func (bashTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	cmd, _ := args["command"].(string)
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return tools.Result{}, fmt.Errorf("command required")
	}
	// Denylist scan — case sensitive (most patterns are case sensitive),
	// tapi compare ke lowered untuk catch `RM -RF /` style.
	lower := strings.ToLower(cmd)
	for _, p := range shellDenyPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return tools.Result{}, fmt.Errorf("shell: blocked dangerous pattern %q", p)
		}
	}

	// Working dir resolution.
	shared := tools.FromSharedDir(ctx)
	if shared == "" {
		return tools.Result{}, fmt.Errorf("shell: shared dir not in context")
	}
	workDir := shared
	if wd, _ := args["working_dir"].(string); wd != "" {
		wd = strings.TrimSpace(wd)
		// Reject absolute path + parent traversal.
		if filepath.IsAbs(wd) {
			return tools.Result{}, fmt.Errorf("shell: working_dir must be relative")
		}
		joined := filepath.Join(shared, wd)
		// Defense in depth: ensure joined stays under shared.
		rel, rerr := filepath.Rel(shared, joined)
		if rerr != nil || strings.HasPrefix(rel, "..") {
			return tools.Result{}, fmt.Errorf("shell: working_dir escapes shared")
		}
		workDir = joined
	}

	// Timeout.
	timeoutSec := 20
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSec = int(v)
	}
	if vs, ok := args["timeout_seconds"].(int); ok && vs > 0 {
		timeoutSec = vs
	}
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	if timeoutSec > 60 {
		timeoutSec = 60
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Pick shell per OS.
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.CommandContext(runCtx, "cmd", "/C", cmd)
	default:
		c = exec.CommandContext(runCtx, "/bin/sh", "-c", cmd)
	}
	c.Dir = workDir
	c.Env = scrubEnv() // strip sensitive vars (tokens) sebelum spawn
	// Section 12 phase 3: enforce memory limit via OS-specific helper.
	// Linux: RLIMIT_AS via `ulimit -v` prepend. Windows/macOS: no-op.
	applyMemLimit(c, cmd)

	var outBuf, errBuf bytes.Buffer
	const outCap = 64 * 1024
	c.Stdout = &capWriter{buf: &outBuf, cap: outCap}
	c.Stderr = &capWriter{buf: &errBuf, cap: outCap}

	t0 := time.Now()
	runErr := c.Run()
	elapsed := time.Since(t0).Milliseconds()

	exitCode := 0
	if runErr != nil {
		// Timeout detection.
		if runCtx.Err() == context.DeadlineExceeded {
			return tools.Result{Output: map[string]any{
				"stdout":      outBuf.String(),
				"stderr":      errBuf.String() + "\n[timeout exceeded]",
				"exit_code":   124,
				"truncated":   outBuf.Len() >= outCap || errBuf.Len() >= outCap,
				"duration_ms": elapsed,
			}}, nil
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return tools.Result{}, fmt.Errorf("shell exec: %w", runErr)
		}
	}

	return tools.Result{Output: map[string]any{
		"stdout":      outBuf.String(),
		"stderr":      errBuf.String(),
		"exit_code":   exitCode,
		"truncated":   outBuf.Len() >= outCap || errBuf.Len() >= outCap,
		"duration_ms": elapsed,
	}}, nil
}

// capWriter — io.Writer yang buang sisa setelah cap bytes ditulis. Aman
// dipakai bersama exec.Cmd.Stdout/Stderr.
type capWriter struct {
	buf *bytes.Buffer
	cap int
}

func (c *capWriter) Write(p []byte) (int, error) {
	remaining := c.cap - c.buf.Len()
	if remaining <= 0 {
		return len(p), nil // pretend wrote, discard
	}
	if len(p) <= remaining {
		return c.buf.Write(p)
	}
	_, _ = c.buf.Write(p[:remaining])
	_, _ = io.WriteString(c.buf, "\n[...truncated]")
	return len(p), nil
}

// scrubEnv — strip sensitive vars sebelum spawn child. Whitelist approach
// kalau ngga ada kebutuhan inherit env tertentu. Phase 1c: minimal —
// inherit PATH/HOME/LANG saja (Linux/macOS) dan SystemRoot/Path/TEMP
// (Windows). Token/credential JANGAN forward — tool dedicated yg pakai.
func scrubEnv() []string {
	wantUnix := []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM"}
	wantWin := []string{"SystemRoot", "Path", "TEMP", "TMP", "USERPROFILE"}
	want := wantUnix
	if runtime.GOOS == "windows" {
		want = wantWin
	}
	out := make([]string, 0, len(want))
	envFn := func(k string) string {
		// Use os.Getenv via helper to avoid extra import noise.
		return getEnv(k)
	}
	for _, k := range want {
		if v := envFn(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// getEnv — wrapper supaya bisa di-mock di test future.
func getEnv(k string) string {
	return os.Getenv(k)
}
