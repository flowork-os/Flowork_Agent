// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

package scanner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type RunOptions struct {
	Target string

	OnlyAuditors []string

	MaxFileBytes int64
}

type RunResult struct {
	Target       string    `json:"target"`
	FilesScanned int       `json:"files_scanned"`
	BytesScanned int64     `json:"bytes_scanned"`
	Findings     []Finding `json:"findings"`
}

var scannableExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
	".sh": true, ".rb": true, ".java": true, ".kt": true,
	".c": true, ".cpp": true, ".h": true, ".rs": true, ".php": true,
	".yaml": true, ".yml": true, ".json": true, ".env": true, ".toml": true,
}

func Run(opts RunOptions) (RunResult, error) {
	if opts.Target == "" {
		return RunResult{}, fmt.Errorf("target required")
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = 2 * 1024 * 1024
	}
	res := RunResult{Target: opts.Target}

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

		runOnFile(opts.Target, &res, chosen, opts.MaxFileBytes)
		return res, nil
	}

	walkErr := filepath.Walk(opts.Target, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {

			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "vendor" || base == "__pycache__" ||
				base == ".work" {

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
			return io.EOF
		}
		return nil
	})
	if walkErr != nil && walkErr != io.EOF {
		return res, fmt.Errorf("walk: %w", walkErr)
	}
	return res, nil
}

func runOnFile(path string, res *RunResult, auditors map[string]AuditFunc, maxBytes int64) {

	if strings.Contains(filepath.ToSlash(path), "internal/scanner/auditors") {
		return
	}
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
	lines := strings.Split(content, "\n")
	for _, f := range auditors {
		for _, fd := range f(path, content) {
			// Suppression: skip temuan di baris ber-marker `// scanner:ignore`
			// (atau `nosec`) di baris itu sendiri atau baris tepat di atasnya.
			if suppressedAt(lines, fd.LineNumber) {
				continue
			}
			res.Findings = append(res.Findings, fd)
		}
	}
}

// marker suppression. Standar industri (mirip gosec #nosec / nolint).
func suppressedAt(lines []string, ln int) bool {
	has := func(i int) bool {
		if i < 1 || i > len(lines) {
			return false
		}
		return strings.Contains(lines[i-1], "scanner:ignore") || strings.Contains(lines[i-1], "nosec")
	}
	return has(ln) || has(ln-1)
}

func Names() []string {
	out := make([]string, 0, len(Auditors))
	for k := range Auditors {
		out = append(out, k)
	}

	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
