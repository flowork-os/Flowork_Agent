// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 25 phase 1 runner — walk target path, dispatch each
//   file ke semua auditor, aggregate findings. Phase 2 (parallel
//   goroutine per auditor, language-specific subset .py/.go/.js,
//   incremental rescan) → tambah file baru.
//
// runner.go — Section 25 phase 1: filesystem walker + auditor dispatch.

package scanner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RunOptions — scan input.
type RunOptions struct {
	Target string // absolute path file atau dir
	// Optional auditor filter (empty = run all).
	OnlyAuditors []string
	// Skip file > N bytes. 0 = default 2MB.
	MaxFileBytes int64
}

// RunResult — aggregated output untuk DB persist + API response.
type RunResult struct {
	Target       string    `json:"target"`
	FilesScanned int       `json:"files_scanned"`
	BytesScanned int64     `json:"bytes_scanned"`
	Findings     []Finding `json:"findings"`
}

// Scannable extensions.
var scannableExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
	".sh": true, ".rb": true, ".java": true, ".kt": true,
	".c": true, ".cpp": true, ".h": true, ".rs": true, ".php": true,
	".yaml": true, ".yml": true, ".json": true, ".env": true, ".toml": true,
}

// Run — walk target, dispatch auditors per file. Limit results 5000.
func Run(opts RunOptions) (RunResult, error) {
	if opts.Target == "" {
		return RunResult{}, fmt.Errorf("target required")
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = 2 * 1024 * 1024
	}
	res := RunResult{Target: opts.Target}

	// Build auditor list.
	chosen := map[string]AuditFunc{}
	if len(opts.OnlyAuditors) == 0 {
		for k, v := range Auditors {
			chosen[k] = v
		}
	} else {
		for _, name := range opts.OnlyAuditors {
			if f, ok := Auditors[name]; ok {
				chosen[name] = f
			}
		}
	}

	info, err := os.Stat(opts.Target)
	if err != nil {
		return res, fmt.Errorf("stat: %w", err)
	}
	if !info.IsDir() {
		// Single file scan.
		runOnFile(opts.Target, &res, chosen, opts.MaxFileBytes)
		return res, nil
	}

	walkErr := filepath.Walk(opts.Target, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			// Skip common noise dirs.
			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "vendor" || base == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !scannableExt[ext] {
			return nil
		}
		if info.Size() > opts.MaxFileBytes {
			return nil
		}
		runOnFile(path, &res, chosen, opts.MaxFileBytes)
		if len(res.Findings) > 5000 {
			return io.EOF // graceful stop
		}
		return nil
	})
	if walkErr != nil && walkErr != io.EOF {
		return res, fmt.Errorf("walk: %w", walkErr)
	}
	return res, nil
}

func runOnFile(path string, res *RunResult, auditors map[string]AuditFunc, maxBytes int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if int64(len(data)) > maxBytes {
		return
	}
	content := string(data)
	res.FilesScanned++
	res.BytesScanned += int64(len(data))
	for _, f := range auditors {
		res.Findings = append(res.Findings, f(path, content)...)
	}
}

// Names — return registered auditor names sorted.
func Names() []string {
	out := make([]string, 0, len(Auditors))
	for k := range Auditors {
		out = append(out, k)
	}
	// Manual sort (avoid extra import for tiny list).
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
