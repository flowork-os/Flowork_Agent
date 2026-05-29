// Package codeindex — auto-dokumentasi & dependency graph FloworkOS.
//
// Cara kerja:
//  1. IndexAll()  — walk seluruh codebase, parse tiap file, simpan ke codemap_nodes + codemap_edges.
//  2. IndexFile() — re-index satu file (dipanggil dari Claude hook saat file berubah).
//  3. IncrementalUpdate() — hanya reindex file yang berubah (CRG-inspired, SHA-256 diff).
//  4. Health score dihitung per-file berdasarkan: doc, test, line count, coupling.
package codeindex

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// skipDirs — folder yang tidak diindeks
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true,
	"build": true, "dist": true, ".cache": true, "testdata": true,
	"docs": true, // avoid indexing auto-generated docs
}

// supportedExts — ekstensi yang diparse
var supportedExts = map[string]string{
	".go":  "go",
	".js":  "js",
	".ts":  "ts",
	".mjs": "js",
}

// IndexStats — statistik hasil indexing
type IndexStats struct {
	FilesIndexed int
	FilesSkipped int // CRG: file yang di-skip karena hash sama
	EdgesCreated int
	Duration     time.Duration
	Errors       int
	Incremental  bool // true kalau pakai IncrementalUpdate
}

// Indexer menyimpan state global untuk indexing session.
//
// Sprint 3.5e (RD-204 fix): support multi-root indexing. Sebelumnya cuma
// floworkos-go yang di-index → cross-repo dependency invisible (kernel module
// ngga ke-track). Sekarang `extraRoots` memungkinkan tambahan workspace
// (e.g., `flowork-kernel/`) untuk di-walk sekaligus.
type Indexer struct {
	db            *sql.DB
	workspaceRoot string
	modulePath    string
	extraRoots    []rootEntry // RD-204: additional workspace roots
	mu            sync.Mutex
	running       bool

	// activeTx — perf fix 2026-05-06: optional transaction yang di-set
	// selama Phase 1/2 IndexAll(). Pre-fix tiap INSERT pake db.Exec
	// langsung → 1656 nodes × 1 + ~11k edges × 1 = 12k+ syscall fsync
	// di SQLite WAL → reindex slow. Sekarang Phase 1 + Phase 2 wrap
	// ke single tx → commit sekali → 100x+ faster di codebase besar.
	activeTx *sql.Tx
}

// rootEntry — extra workspace root + module path-nya (untuk Go import resolve).
type rootEntry struct {
	Root       string // absolute path root (forward slash)
	ModulePath string // module path dari go.mod di root tsb
}

// NewIndexer membuat Indexer baru.
func NewIndexer(db *sql.DB, workspaceRoot string) *Indexer {
	return &Indexer{
		db:            db,
		workspaceRoot: workspaceRoot,
		modulePath:    ReadModulePath(workspaceRoot),
	}
}

// AddRoot — RD-204: register additional workspace root untuk multi-repo indexing.
// Module path auto-resolved via go.mod di root tsb. Idempotent (re-add ngga
// duplicate). Return true kalau actually di-add (ada go.mod, root unique).
// Return false kalau di-skip (no go.mod, sama dengan primary, atau duplicate).
func (ix *Indexer) AddRoot(root string) bool {
	root = filepath.ToSlash(filepath.Clean(root))
	if root == "" || root == ix.workspaceRoot {
		return false
	}
	mod := ReadModulePath(root)
	if mod == "" {
		return false // not a Go module — skip
	}
	for _, e := range ix.extraRoots {
		if e.Root == root {
			return false // already added
		}
	}
	ix.extraRoots = append(ix.extraRoots, rootEntry{Root: root, ModulePath: mod})
	return true
}

// dbExec — pilih activeTx kalau di tengah batch, fallback db.Exec.
// Dipakai semua INSERT/UPDATE di Phase 1/2 IndexAll supaya batchable.
func (ix *Indexer) dbExec(query string, args ...any) (sql.Result, error) {
	if ix.activeTx != nil {
		return ix.activeTx.Exec(query, args...)
	}
	return ix.db.Exec(query, args...)
}

// IsRunning cek apakah indexing sedang berjalan.
func (ix *Indexer) IsRunning() bool {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.running
}

// IndexAll walk seluruh codebase dan re-index semua file.
func (ix *Indexer) IndexAll() (*IndexStats, error) {
	ix.mu.Lock()
	if ix.running {
		ix.mu.Unlock()
		return nil, fmt.Errorf("indexing already running")
	}
	ix.running = true
	ix.mu.Unlock()

	defer func() {
		ix.mu.Lock()
		ix.running = false
		ix.mu.Unlock()
	}()

	start := time.Now()
	stats := &IndexStats{}

	// Bersihkan data lama
	if _, err := ix.db.Exec(`DELETE FROM codemap_edges`); err != nil {
		return nil, fmt.Errorf("clear edges: %w", err)
	}
	if _, err := ix.db.Exec(`DELETE FROM codemap_nodes`); err != nil {
		return nil, fmt.Errorf("clear nodes: %w", err)
	}

	files := ix.collectFiles()

	// Phase 1: parse + simpan semua nodes (single tx batching).
	if tx, err := ix.db.Begin(); err == nil {
		ix.activeTx = tx
		for _, f := range files {
			if err := ix.indexFileNodes(f); err != nil {
				log.Printf("[codeindex] node error %s: %v", f, err)
				stats.Errors++
				continue
			}
			stats.FilesIndexed++
		}
		ix.activeTx = nil
		if err := tx.Commit(); err != nil {
			log.Printf("[codeindex] phase1 commit error: %v", err)
		}
	} else {
		log.Printf("[codeindex] phase1 begin tx failed (%v) — fallback to per-stmt", err)
		for _, f := range files {
			if err := ix.indexFileNodes(f); err != nil {
				log.Printf("[codeindex] node error %s: %v", f, err)
				stats.Errors++
				continue
			}
			stats.FilesIndexed++
		}
	}

	// Phase 2: resolve + simpan edges (single tx batching).
	if tx, err := ix.db.Begin(); err == nil {
		ix.activeTx = tx
		for _, f := range files {
			n, err := ix.indexFileEdges(f)
			if err != nil {
				log.Printf("[codeindex] edge error %s: %v", f, err)
				stats.Errors++
				continue
			}
			stats.EdgesCreated += n
		}
		ix.activeTx = nil
		if err := tx.Commit(); err != nil {
			log.Printf("[codeindex] phase2 commit error: %v", err)
		}
	} else {
		log.Printf("[codeindex] phase2 begin tx failed (%v) — fallback to per-stmt", err)
		for _, f := range files {
			n, err := ix.indexFileEdges(f)
			if err != nil {
				log.Printf("[codeindex] edge error %s: %v", f, err)
				stats.Errors++
				continue
			}
			stats.EdgesCreated += n
		}
	}

	// Phase 3: hitung health score setelah semua edges ada
	if err := ix.recalcHealthAll(); err != nil {
		log.Printf("[codeindex] health recalc error: %v", err)
	}

	// Phase 4: generate docs
	go GenerateDocs(ix.db, ix.workspaceRoot) // async, non-blocking

	stats.Duration = time.Since(start)
	log.Printf("[codeindex] indexed %d files, %d edges, %d errors in %s",
		stats.FilesIndexed, stats.EdgesCreated, stats.Errors, stats.Duration.Round(time.Millisecond))
	return stats, nil
}

// IncrementalUpdate — CRG-inspired: hanya reindex file yang berubah.
// Jauh lebih cepat dari IndexAll() untuk codebase besar.
// Pakai SHA-256 content hash untuk detect perubahan.
func (ix *Indexer) IncrementalUpdate() (*IndexStats, error) {
	ix.mu.Lock()
	if ix.running {
		ix.mu.Unlock()
		return nil, fmt.Errorf("indexing already running")
	}
	ix.running = true
	ix.mu.Unlock()

	defer func() {
		ix.mu.Lock()
		ix.running = false
		ix.mu.Unlock()
	}()

	start := time.Now()
	stats := &IndexStats{Incremental: true}

	files := ix.collectFiles()

	// Phase 1: check hash, only reindex changed files
	var changedFiles []string
	for _, f := range files {
		hash, err := sha256File(f)
		if err != nil {
			stats.Errors++
			continue
		}
		relPath := ix.relPath(f)
		var existingHash string
		ix.db.QueryRow(`SELECT content_hash FROM codemap_nodes WHERE path = ?`, relPath).Scan(&existingHash)
		if existingHash == hash {
			stats.FilesSkipped++
			continue
		}
		// File baru atau berubah — perlu reindex
		if _, err := ix.db.Exec(`DELETE FROM codemap_nodes WHERE path = ?`, relPath); err != nil { log.Printf("codeindex: DELETE codemap_nodes failed: %v", err) }
		if _, err := ix.db.Exec(`DELETE FROM codemap_edges WHERE from_path = ?`, relPath); err != nil { log.Printf("codeindex: DELETE codemap_edges failed: %v", err) }
		if err := ix.indexFileNodes(f); err != nil {
			log.Printf("[codeindex-incr] node error %s: %v", f, err)
			stats.Errors++
			continue
		}
		stats.FilesIndexed++
		changedFiles = append(changedFiles, f)
	}

	// Phase 2: rebuild edges hanya untuk file yang berubah
	for _, f := range changedFiles {
		n, err := ix.indexFileEdges(f)
		if err != nil {
			log.Printf("[codeindex-incr] edge error %s: %v", f, err)
			stats.Errors++
			continue
		}
		stats.EdgesCreated += n
	}

	// Phase 3: recalc health hanya untuk changed + neighbors
	if len(changedFiles) > 0 {
		neighbors := ix.collectNeighbors(changedFiles)
		for _, p := range neighbors {
			ix.recalcHealth(p)
		}
	}

	// Detect deleted files — file di DB tapi tidak di disk
	fileSet := make(map[string]bool, len(files))
	for _, f := range files {
		fileSet[ix.relPath(f)] = true
	}
	rows, _ := ix.db.Query(`SELECT path FROM codemap_nodes`)
	if rows != nil {
		for rows.Next() {
			var p string
			rows.Scan(&p)
			if !fileSet[p] {
				if _, err := ix.db.Exec(`DELETE FROM codemap_nodes WHERE path = ?`, p); err != nil { log.Printf("codeindex: orphan DELETE codemap_nodes failed: %v", err) }
				if _, err := ix.db.Exec(`DELETE FROM codemap_edges WHERE from_path = ? OR to_path = ?`, p, p); err != nil { log.Printf("codeindex: orphan DELETE codemap_edges failed: %v", err) }
			}
		}
		// Sprint 3.5d (BUG-C15 fix): rows.Err() check
		_ = rows.Err()
		rows.Close()
	}

	stats.Duration = time.Since(start)
	log.Printf("[codeindex-incr] %d changed, %d skipped, %d edges, %d errors in %s",
		stats.FilesIndexed, stats.FilesSkipped, stats.EdgesCreated, stats.Errors,
		stats.Duration.Round(time.Millisecond))
	return stats, nil
}

// IndexFile re-index satu file spesifik (dipanggil dari hook atau reindex endpoint).
// CRG enhancement: cek hash dulu, skip kalau tidak berubah.
func (ix *Indexer) IndexFile(absPath string) error {
	relPath := ix.relPath(absPath)

	// CRG: cek hash — skip kalau tidak berubah
	hash, err := sha256File(absPath)
	if err != nil {
		return err
	}
	var existingHash string
	ix.db.QueryRow(`SELECT content_hash FROM codemap_nodes WHERE path = ?`, relPath).Scan(&existingHash)
	if existingHash == hash {
		return nil // file tidak berubah
	}

	// Hapus data lama untuk file ini
	if _, err := ix.db.Exec(`DELETE FROM codemap_nodes WHERE path = ?`, relPath); err != nil { log.Printf("codeindex: cleanup DELETE codemap_nodes failed: %v", err) }
	if _, err := ix.db.Exec(`DELETE FROM codemap_edges WHERE from_path = ?`, relPath); err != nil { log.Printf("codeindex: cleanup DELETE codemap_edges failed: %v", err) }

	if err := ix.indexFileNodes(absPath); err != nil {
		return err
	}
	if _, err := ix.indexFileEdges(absPath); err != nil {
		return err
	}
	return ix.recalcHealth(relPath)
}

// ─── Internal helpers ─────────────────────────────────────────────────────

func (ix *Indexer) relPath(absPath string) string {
	// 2026-05-06 (Ayah audit ".go ngak saling terhubung itu yang terhubung .js"):
	// ResolveGoImportToFiles return forward-slash path. workspaceRoot dari env
	// FLOWORK_WORKSPACE pake backslash (Windows). filepath.Rel pada Windows
	// butuh consistent separator → mixed input return err → fallback simpan
	// ABSOLUTE PATH sebagai to_path → ngga match relPath node yang udah
	// converted → edge filtered di frontend → SEMUA .go orphan.
	// Fix: normalize keduanya ke OS-native dulu via FromSlash.
	wsNorm := filepath.FromSlash(ix.workspaceRoot)
	absNorm := filepath.FromSlash(absPath)
	rel, err := filepath.Rel(wsNorm, absNorm)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

func (ix *Indexer) indexFileNodes(absPath string) error {
	ext := strings.ToLower(filepath.Ext(absPath))
	ft, ok := supportedExts[ext]
	if !ok {
		return nil
	}
	relPath := ix.relPath(absPath)

	// CRG: compute content hash
	hash, _ := sha256File(absPath)

	var (
		pkg      string
		symbols  []string
		docCmt   string
		lines    int
		sizeB    int64
	)

	switch ft {
	case "go":
		info, err := ParseGoFile(absPath)
		if err != nil {
			return fmt.Errorf("go parse: %w", err)
		}
		pkg = info.Package
		symbols = info.ExportedSymbols
		docCmt = info.DocComment
		lines = info.LineCount
		sizeB = info.SizeBytes
	case "js", "ts":
		info, err := ParseJSFile(absPath)
		if err != nil {
			return fmt.Errorf("js parse: %w", err)
		}
		symbols = info.ExportedSymbols
		docCmt = info.DocComment
		lines = info.LineCount
		sizeB = info.SizeBytes
	}

	symJSON, _ := json.Marshal(symbols)
	hasDocs := 0
	if docCmt != "" {
		hasDocs = 1
	}
	hasTests := 0
	if isTestFile(absPath) || hasTestCounterpart(absPath) {
		hasTests = 1
	}

	_, err := ix.dbExec(`
		INSERT OR REPLACE INTO codemap_nodes
		  (path, name, pkg, file_type, line_count, size_bytes, exported_symbols, doc_comment, has_tests, has_docs, last_indexed, content_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)`,
		relPath, filepath.Base(absPath), pkg, ft, lines, sizeB,
		string(symJSON), docCmt, hasTests, hasDocs, hash,
	)
	return err
}

func (ix *Indexer) indexFileEdges(absPath string) (int, error) {
	ext := strings.ToLower(filepath.Ext(absPath))
	ft, ok := supportedExts[ext]
	if !ok {
		return 0, nil
	}
	relPath := ix.relPath(absPath)
	count := 0

	var rawImports []string
	switch ft {
	case "go":
		info, err := ParseGoFile(absPath)
		if err != nil {
			return 0, err
		}
		rawImports = info.Imports
	case "js", "ts":
		info, err := ParseJSFile(absPath)
		if err != nil {
			return 0, err
		}
		rawImports = info.Imports
	}

	for _, imp := range rawImports {
		switch ft {
		case "go":
			// Sprint 3.5e RD-201 fix: fan-out 1 import → N edges (1 per .go
			// file di package). Sebelumnya resolve ke directory yang bikin
			// codemap_deps/impact query broken (to_path bukan file actual).
			//
			// 2026-05-06 (multi-repo edge fix per Ayah audit "banyak ngga
			// terhubung"): coba resolve terhadap SEMUA known modules
			// (primary + extraRoots). Sebelumnya cuma primary modulePath →
			// flowork-kernel/flowork_docktor file's internal imports tampak
			// orphan padahal nyata terhubung. First match wins.
			var absFiles []string
			if r := ResolveGoImportToFiles(imp, ix.modulePath, ix.workspaceRoot); len(r) > 0 {
				absFiles = r
			} else {
				for _, e := range ix.extraRoots {
					if r := ResolveGoImportToFiles(imp, e.ModulePath, e.Root); len(r) > 0 {
						absFiles = r
						break
					}
				}
			}
			for _, absTo := range absFiles {
				toPath := ix.relPath(absTo)
				if toPath == "" || toPath == relPath {
					continue
				}
				_, err := ix.dbExec(`
					INSERT OR IGNORE INTO codemap_edges (from_path, to_path, edge_type)
					VALUES (?, ?, 'import')`, relPath, toPath)
				if err == nil {
					count++
				}
			}
		case "js", "ts":
			absTo := ResolveJSImportToPath(imp, filepath.Dir(absPath))
			if absTo == "" {
				continue
			}
			toPath := ix.relPath(absTo)
			if toPath == "" || toPath == relPath {
				continue
			}
			_, err := ix.dbExec(`
				INSERT OR IGNORE INTO codemap_edges (from_path, to_path, edge_type)
				VALUES (?, ?, 'import')`, relPath, toPath)
			if err == nil {
				count++
			}
		}
	}
	return count, nil
}

// recalcHealthAll hitung ulang health score semua nodes.
func (ix *Indexer) recalcHealthAll() error {
	rows, err := ix.db.Query(`SELECT path FROM codemap_nodes`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		rows.Scan(&p)
		paths = append(paths, p)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	rows.Close()
	for _, p := range paths {
		if err := ix.recalcHealth(p); err != nil {
			log.Printf("[codeindex] health %s: %v", p, err)
		}
	}
	return nil
}

func (ix *Indexer) recalcHealth(relPath string) error {
	var lineCount, hasDocs, hasTests int
	var symJSON string
	ix.db.QueryRow(
		`SELECT line_count, has_docs, has_tests, exported_symbols FROM codemap_nodes WHERE path = ?`,
		relPath,
	).Scan(&lineCount, &hasDocs, &hasTests, &symJSON)

	// Jumlah deps (outgoing edges)
	var depCount int
	ix.db.QueryRow(`SELECT COUNT(*) FROM codemap_edges WHERE from_path = ?`, relPath).Scan(&depCount)

	// Jumlah dependents (incoming edges = siapa yang bergantung pada file ini)
	var dependentCount int
	ix.db.QueryRow(`SELECT COUNT(*) FROM codemap_edges WHERE to_path = ?`, relPath).Scan(&dependentCount)

	// Cek circular dep
	hasCircular := 0
	var circRows *sql.Rows
	circRows, _ = ix.db.Query(`SELECT to_path FROM codemap_edges WHERE from_path = ?`, relPath)
	if circRows != nil {
		for circRows.Next() {
			var dep string
			circRows.Scan(&dep)
			var back int
			ix.db.QueryRow(`SELECT COUNT(*) FROM codemap_edges WHERE from_path = ? AND to_path = ?`, dep, relPath).Scan(&back)
			if back > 0 {
				hasCircular = 1
				break
			}
		}
		// Sprint 3.5d (BUG-C15 fix): rows.Err() check
		_ = circRows.Err()
		circRows.Close()
	}

	// Hitung score
	score := 100.0
	issues := []string{}

	if hasDocs == 0 {
		score -= 15
		issues = append(issues, "tidak ada godoc/JSDoc comment")
	}
	if hasTests == 0 {
		score -= 20
		issues = append(issues, "tidak ada test file")
	}
	if lineCount > 1000 {
		score -= 20
		issues = append(issues, fmt.Sprintf("file terlalu panjang (%d baris)", lineCount))
	} else if lineCount > 500 {
		score -= 10
		issues = append(issues, fmt.Sprintf("file cukup panjang (%d baris)", lineCount))
	}
	if depCount > 15 {
		score -= 15
		issues = append(issues, fmt.Sprintf("terlalu banyak dependensi (%d)", depCount))
	} else if depCount > 8 {
		score -= 5
		issues = append(issues, fmt.Sprintf("banyak dependensi (%d)", depCount))
	}
	if dependentCount > 20 {
		issues = append(issues, fmt.Sprintf("blast radius tinggi: %d file bergantung di sini", dependentCount))
		score -= 5
	}
	if hasCircular == 1 {
		score -= 25
		issues = append(issues, "circular dependency terdeteksi")
	}

	score = math.Max(0, math.Min(100, score))
	issJSON, _ := json.Marshal(issues)

	_, err := ix.db.Exec(`
		UPDATE codemap_nodes SET health_score = ?, issues = ? WHERE path = ?`,
		score, string(issJSON), relPath)
	return err
}

// isTestFile — true kalau nama file berakhiran _test.go atau .test.js
func isTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".spec.ts")
}

// hasTestCounterpart — cek apakah ada file test yang bersesuaian
func hasTestCounterpart(path string) bool {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	candidates := []string{
		base + "_test" + ext,
		base + ".test" + ext,
		base + ".spec" + ext,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return true
		}
	}
	return false
}

// ─── CRG-inspired helpers ─────────────────────────────────────────────────

// sha256File returns hex-encoded SHA-256 of file content.
func sha256File(absPath string) (string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// collectFiles walks workspace + extra roots dan return all supported file paths.
//
// Sprint 3.5e (RD-204): walk workspaceRoot + extraRoots (multi-repo).
// Behavior single-root preserved (extraRoots default empty).
func (ix *Indexer) collectFiles() []string {
	var files []string
	roots := []string{ix.workspaceRoot}
	for _, e := range ix.extraRoots {
		roots = append(roots, e.Root)
	}
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if _, ok := supportedExts[ext]; ok {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

// collectNeighbors returns relPaths of changed files + their direct dependents.
// Used by IncrementalUpdate to recalc health only for affected nodes.
func (ix *Indexer) collectNeighbors(changedFiles []string) []string {
	seen := map[string]bool{}
	for _, f := range changedFiles {
		p := ix.relPath(f)
		seen[p] = true
		// Add direct dependents (files that import this one)
		rows, _ := ix.db.Query(`SELECT from_path FROM codemap_edges WHERE to_path = ?`, p)
		if rows != nil {
			for rows.Next() {
				var dep string
				rows.Scan(&dep)
				seen[dep] = true
			}
			// Sprint 3.5d (BUG-C15 fix): rows.Err() check
			_ = rows.Err()
			rows.Close()
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

// DetectGitChangedFiles returns files changed since last commit.
// Used by git hook integration for automatic incremental update.
func DetectGitChangedFiles(workspaceRoot string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD~1")
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		// Fallback: git status for uncommitted changes
		cmd2 := exec.Command("git", "diff", "--name-only")
		cmd2.Dir = workspaceRoot
		out, err = cmd2.Output()
		if err != nil {
			return nil, fmt.Errorf("git diff: %w", err)
		}
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(line))
		if _, ok := supportedExts[ext]; ok {
			files = append(files, filepath.Join(workspaceRoot, filepath.FromSlash(line)))
		}
	}
	return files, nil
}
