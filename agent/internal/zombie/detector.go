// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package zombie

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
)

type ScanOptions struct {
	SharedRoot string

	MinAgeDays int
}

type ScanResult struct {
	SymbolsScanned int                     `json:"symbols_scanned"`
	FilesGrepped   int                     `json:"files_grepped"`
	Inserted       int                     `json:"inserted"`
	Findings       []agentdb.ZombieFinding `json:"findings"`
}

func Scan(ctx context.Context, store *agentdb.Store, opts ScanOptions) (ScanResult, error) {
	var res ScanResult
	if opts.MinAgeDays <= 0 {
		opts.MinAgeDays = 30
	}
	if opts.SharedRoot == "" {
		return res, fmt.Errorf("SharedRoot required")
	}

	nodes, err := store.ListCodemapNodes("", "", "", 1000)
	if err != nil {
		return res, err
	}
	res.SymbolsScanned = len(nodes)

	type fileEntry struct {
		path    string
		content string
		modTime time.Time
	}
	var files []fileEntry
	allowedExt := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".rb": true,
	}
	_ = filepath.Walk(opts.SharedRoot, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if !allowedExt[ext] {
			return nil
		}
		if info.Size() > 2*1024*1024 {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		files = append(files, fileEntry{
			path:    p,
			content: string(data),
			modTime: info.ModTime(),
		})
		return nil
	})
	res.FilesGrepped = len(files)

	cutoff := time.Now().AddDate(0, 0, -opts.MinAgeDays)

	for _, n := range nodes {

		callerFiles := 0
		for _, f := range files {
			if strings.HasSuffix(f.path, n.FilePath) && hasIdentifier(f.content, n.Name, true) {
				continue
			}
			if hasIdentifier(f.content, n.Name, false) {
				callerFiles++
			}
		}
		if callerFiles > 0 {
			continue
		}

		tooNew := false
		for _, f := range files {
			if strings.HasSuffix(f.path, n.FilePath) {
				if f.modTime.After(cutoff) {
					tooNew = true
				}
				break
			}
		}
		if tooNew {
			continue
		}
		reason := fmt.Sprintf("no caller found in %d files scanned (older than %d days)",
			len(files), opts.MinAgeDays)
		zf := agentdb.ZombieFinding{
			FilePath:   n.FilePath,
			SymbolName: n.Name,
			SymbolType: n.NodeType,
			Confidence: "medium",
			Reason:     reason,
		}
		id, ierr := store.AddZombieFinding(zf)
		if ierr == nil {
			zf.ID = id
			res.Findings = append(res.Findings, zf)
			res.Inserted++
		}
	}
	return res, nil
}

func hasIdentifier(s, name string, ignoreDefn bool) bool {
	if name == "" {
		return false
	}
	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		idx := 0
		for {
			i := strings.Index(line[idx:], name)
			if i < 0 {
				break
			}
			abs := idx + i
			leftOK := abs == 0 || !isIdentChar(line[abs-1])
			rightOK := abs+len(name) >= len(line) || !isIdentChar(line[abs+len(name)])
			if leftOK && rightOK {
				if ignoreDefn {
					prefix := strings.TrimSpace(line[:abs])
					if strings.HasSuffix(prefix, "func") || strings.HasSuffix(prefix, "type") ||
						strings.HasSuffix(prefix, "var") || strings.HasSuffix(prefix, "const") {
						idx = abs + len(name)
						continue
					}
				}
				return true
			}
			idx = abs + len(name)
		}
	}
	return false
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}
