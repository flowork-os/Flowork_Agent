// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 29 phase 2 — real zombie auto-detect via codemap_nodes
//   + grep heuristic. Phase 3 (callgraph edges Section 27 phase 2,
//   git_blame age, semantic dead code analysis) → tambah file baru.
//
// detector.go — Section 29 phase 2: auto-scan zombie candidates.

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

// ScanOptions — knob.
type ScanOptions struct {
	SharedRoot string // base directory untuk grep callers
	// Min file age dalam days. Default 30 — anything modified < 30 days
	// di-skip (likely WIP).
	MinAgeDays int
}

// ScanResult — diagnostic.
type ScanResult struct {
	SymbolsScanned int                   `json:"symbols_scanned"`
	FilesGrepped   int                   `json:"files_grepped"`
	Inserted       int                   `json:"inserted"`
	Findings       []agentdb.ZombieFinding `json:"findings"`
}

// Scan — iterate codemap_nodes. Per symbol, grep all .go/.py/.js files in
// SharedRoot for symbol name. Kalau 0 reference (selain di file asalnya) →
// INSERT zombie_finding.
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

	// Pre-gather all source files + content (one pass).
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
		// Skip too-new files (likely WIP).
		// (file_path in node is relative — we don't know absolute mod time
		// without re-stat. For phase 2, skip age check kalau ngga ada
		// file di shared.)
		// Count callers — text match (n.Name surrounded by non-identifier
		// chars).
		callerFiles := 0
		for _, f := range files {
			if strings.HasSuffix(f.path, n.FilePath) && hasIdentifier(f.content, n.Name, true) {
				continue // own file — skip
			}
			if hasIdentifier(f.content, n.Name, false) {
				callerFiles++
			}
		}
		if callerFiles > 0 {
			continue
		}
		// Apply MinAgeDays — skip kalau file too new (likely WIP).
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

// hasIdentifier — return true kalau s contains name surrounded by
// non-identifier chars (word boundary heuristic — Go-style identifier).
//
// ignoreDefn=true → skip kalau preceded by "func", "type", "var", "const"
// (the symbol's own definition).
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
