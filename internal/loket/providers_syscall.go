// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: fs.* per-folder isolation. scopedPath enforces BOTH lexical and symlink
//   containment (EvalSymlinks) — the guarantee that "a module touches only its own
//   folder". Removing the symlink resolution re-opens CWE-59 escape.
package loket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// syscallProviders back the high-risk fs.* and exec.run capabilities (§2.C of the
// contract). They are GrantOwner: a module gets them only by DECLARING them in its
// loket.json AND the owner approving — implementing the provider does NOT hand the
// power to anyone. File access is SCOPED to the module's own folder: the kernel
// resolves every path under ModuleDir and refuses any path that escapes it, so a
// module can never read/write outside its own plug-and-play folder.
type syscallProviders struct {
	deps Deps
}

// scopedPath resolves p within the module's own folder and rejects any escape
// ("../", absolute paths outside the folder). This is the isolation guarantee for
// fs.* — enforced by the kernel, not trusted to the module.
func (s *syscallProviders) scopedPath(module, p string) (string, error) {
	if s.deps.ModuleDir == nil {
		return "", fmt.Errorf("module dir resolver unavailable")
	}
	base, err := s.deps.ModuleDir(module)
	if err != nil {
		return "", err
	}
	base = filepath.Clean(base)
	full := filepath.Clean(filepath.Join(base, p))
	// (1) Lexical containment — rejects "../" escapes cheaply.
	if full != base && !strings.HasPrefix(full, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("fs: path escapes module folder")
	}
	// (2) Symlink containment — a lexical check alone is fooled by a symlink INSIDE
	// the folder that points outside it (CWE-59). Resolve symlinks on both the base
	// and the longest existing prefix of the target; if the real path leaves the real
	// base, refuse. This is what makes "a module can only touch its own folder" true.
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		realBase = base
	}
	realBase = filepath.Clean(realBase)
	resolved := resolveExistingPrefix(full)
	if resolved != realBase && !strings.HasPrefix(resolved, realBase+string(os.PathSeparator)) {
		return "", fmt.Errorf("fs: path escapes module folder (symlink)")
	}
	return full, nil
}

// resolveExistingPrefix resolves symlinks in the longest existing ancestor of p,
// then re-appends the not-yet-existing tail (which cannot contain a symlink because
// those components do not exist yet). This lets fs.write target a new file while
// still catching a symlinked parent directory that escapes the module folder.
func resolveExistingPrefix(p string) string {
	cur := p
	var tail []string
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			r = filepath.Clean(r)
			for i := len(tail) - 1; i >= 0; i-- {
				r = filepath.Join(r, tail[i])
			}
			return r
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(p)
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

func (s *syscallProviders) fsRead(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	full, err := s.scopedPath(module, a.Path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, fmt.Errorf("fs.read: %w", err)
	}
	truncated := false
	if len(b) > 8<<20 { // cap 8 MiB
		b = b[:8<<20]
		truncated = true
	}
	return mustJSON(map[string]any{"content": string(b), "bytes": len(b), "truncated": truncated}), nil
}

func (s *syscallProviders) fsWrite(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Append  bool   `json:"append"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	full, err := s.scopedPath(module, a.Path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, fmt.Errorf("fs.write mkdir: %w", err)
	}
	flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if a.Append {
		flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(full, flag, 0o644)
	if err != nil {
		return nil, fmt.Errorf("fs.write: %w", err)
	}
	defer f.Close()
	n, err := f.WriteString(a.Content)
	if err != nil {
		return nil, fmt.Errorf("fs.write: %w", err)
	}
	return mustJSON(map[string]any{"ok": true, "bytes": n}), nil
}

func (s *syscallProviders) fsList(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(args, &a)
	full, err := s.scopedPath(module, a.Path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, fmt.Errorf("fs.list: %w", err)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		size := int64(0)
		if info, ierr := e.Info(); ierr == nil {
			size = info.Size()
		}
		out = append(out, map[string]any{"name": e.Name(), "dir": e.IsDir(), "size": size})
	}
	return mustJSON(map[string]any{"entries": out, "count": len(out)}), nil
}

// execRun runs a bounded command in the module's own folder (§2.C, the highest-risk
// cap — GrantOwner). Bounds: timeout (default 30s, max 120s) + captured output cap.
// The owner-approved grant is the primary control; the cwd is the module folder.
func (s *syscallProviders) execRun(ctx context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Cmd       string   `json:"cmd"`
		Args      []string `json:"args"`
		TimeoutMs int      `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Cmd) == "" {
		return nil, fmt.Errorf("exec.run: cmd required")
	}
	to := time.Duration(a.TimeoutMs) * time.Millisecond
	if to <= 0 || to > 120*time.Second {
		to = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cmd := exec.CommandContext(cctx, a.Cmd, a.Args...)
	if s.deps.ModuleDir != nil {
		if dir, derr := s.deps.ModuleDir(module); derr == nil {
			cmd.Dir = dir
		}
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	code := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return mustJSON(map[string]any{
		"stdout": clipOut(stdout.String()),
		"stderr": clipOut(stderr.String()),
		"code":   code,
	}), nil
}

// clipOut bounds captured command output to 256 KiB.
func clipOut(s string) string {
	if len(s) > 256<<10 {
		return s[:256<<10] + "…(truncated)"
	}
	return s
}
