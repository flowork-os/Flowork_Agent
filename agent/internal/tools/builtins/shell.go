// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

var shellDenyPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"rm -rf ~",
	"rm --no-preserve-root",
	":(){:|:&};:",
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
	"curl -s http",
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

	lower := strings.ToLower(cmd)
	for _, p := range shellDenyPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return tools.Result{}, fmt.Errorf("shell: blocked dangerous pattern %q", p)
		}
	}

	shared := tools.FromSharedDir(ctx)
	if shared == "" {
		return tools.Result{}, fmt.Errorf("shell: shared dir not in context")
	}
	workDir := shared
	if wd, _ := args["working_dir"].(string); wd != "" {
		wd = strings.TrimSpace(wd)

		if filepath.IsAbs(wd) {
			return tools.Result{}, fmt.Errorf("shell: working_dir must be relative")
		}
		joined := filepath.Join(shared, wd)

		rel, rerr := filepath.Rel(shared, joined)
		if rerr != nil || strings.HasPrefix(rel, "..") {
			return tools.Result{}, fmt.Errorf("shell: working_dir escapes shared")
		}
		workDir = joined
	}

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

	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.CommandContext(runCtx, "cmd", "/C", cmd)
	default:
		c = exec.CommandContext(runCtx, "/bin/sh", "-c", cmd)
	}
	c.Dir = workDir
	c.Env = scrubEnv()

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

type capWriter struct {
	buf *bytes.Buffer
	cap int
}

func (c *capWriter) Write(p []byte) (int, error) {
	remaining := c.cap - c.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) <= remaining {
		return c.buf.Write(p)
	}
	_, _ = c.buf.Write(p[:remaining])
	_, _ = io.WriteString(c.buf, "\n[...truncated]")
	return len(p), nil
}

func scrubEnv() []string {
	wantUnix := []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM"}
	wantWin := []string{"SystemRoot", "Path", "TEMP", "TMP", "USERPROFILE"}
	want := wantUnix
	if runtime.GOOS == "windows" {
		want = wantWin
	}
	out := make([]string, 0, len(want))
	envFn := func(k string) string {

		return getEnv(k)
	}
	for _, k := range want {
		if v := envFn(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func getEnv(k string) string {
	return os.Getenv(k)
}
