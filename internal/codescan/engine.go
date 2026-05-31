// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Background code scanner (fsnotify debounce, skip-noise, single-file
//   scan, persist run/findings, audit + Telegram notify on critical/high).
//   Auto-start di main() — one-click. E2E verified (decoy SQLi → run fail
//   crit=1 + audit scanner_finding + notify path).
//
// Package codescan — background code scanner. Watch source repo + kode yang
// dibikin AI (/shared/<id>/tools/) via fsnotify; pas ada file kode berubah
// (lo/AI ngedit/update) → auto-scan file itu pakai auditor scanner (Section 25)
// → deteksi bug/celah dari perubahan. Hasil di-persist sebagai scanner run
// (scan_type "auto:*", muncul di tab Scanner), critical/high → audit log +
// notify owner (Telegram).
//
// Tujuan: mastiin tiap perbaikan/update ngga malah buka celah baru — kayak
// CI lokal yang jalan sendiri di belakang.
package codescan

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/scanner"
)

// Callbacks di-inject dari main.go.
type AgentEnumerator func() []string
type StoreOpener func(agentID string) (*agentdb.Store, error)
type SharedDirFunc func(agentID string) (string, error)

// Notifier — push ringkasan temuan critical/high ke owner (mis. Telegram).
type Notifier func(ctx context.Context, title, body string) error

// primaryAgent — store tujuan persist scanner run (dashboard tab Scanner pakai id=mr-flow).
const primaryAgent = "mr-flow"

const (
	debounceWait = 1500 * time.Millisecond
	maxBatch     = 50
)

// skipDirs — folder yang ngga di-watch (noise / bukan source).
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "referensifile": true,
	"web": true, "bin": true, ".scratch": true, "sdk": true, "__pycache__": true,
	"workspace": true, // state.db churn — bukan source code
}

// scanExt — ekstensi yang di-scan.
var scanExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".sh": true,
	".rb": true, ".rs": true, ".php": true, ".java": true, ".c": true, ".cpp": true,
}

// Engine — top-level background scanner.
type Engine struct {
	enum       AgentEnumerator
	opener     StoreOpener
	sharedDir  SharedDirFunc
	notifier   Notifier
	sourceRoot string

	watcher *fsnotify.Watcher
	mu      sync.Mutex
	pending map[string]bool
	timer   *time.Timer
}

// New bikin Engine. sourceRoot = root source repo yang di-watch (cwd).
func New(enum AgentEnumerator, opener StoreOpener, sharedDir SharedDirFunc, notifier Notifier, sourceRoot string) *Engine {
	return &Engine{
		enum: enum, opener: opener, sharedDir: sharedDir, notifier: notifier,
		sourceRoot: sourceRoot, pending: map[string]bool{},
	}
}

// Start — pasang fsnotify watcher + jalanin loop. Stop via ctx cancel.
func (e *Engine) Start(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[codescan] watcher init failed: %v (background scan disabled)", err)
		return
	}
	e.watcher = w
	added := 0
	for _, d := range e.watchDirs() {
		if aerr := w.Add(d); aerr == nil {
			added++
		}
	}
	log.Printf("[codescan] engine started — watching %d dirs (source=%s + shared tools)", added, e.sourceRoot)
	go e.loop(ctx)
	// Baseline scan repo sekali pas boot → radar langsung keisi (state terkini),
	// ga nunggu file berubah dulu. Goroutine biar ga nge-block boot.
	go e.scanRepoBaseline(ctx)
}

// scanRepoBaseline — SATU run konsolidasi atas seluruh sourceRoot (bukan per-file)
// biar radar punya data awal tanpa nge-flood scan log. Ga notify Telegram (biar
// ga spam tiap restart) — cukup persist + audit kalau ada critical/high.
func (e *Engine) scanRepoBaseline(ctx context.Context) {
	if e.sourceRoot == "" {
		return
	}
	store, err := e.opener(primaryAgent)
	if err != nil {
		log.Printf("[codescan] baseline open store: %v", err)
		return
	}
	defer store.Close()
	runID, ierr := store.InsertScannerRun("auto:startup", "repo (baseline)")
	if ierr != nil {
		return
	}
	res, rerr := scanner.Run(scanner.RunOptions{Target: e.sourceRoot})
	if rerr != nil {
		_ = store.FinishScannerRun(runID, 0, 0, "fail")
		return
	}
	dbF := make([]agentdb.ScannerFinding, 0, len(res.Findings))
	crit, high := 0, 0
	for _, f := range res.Findings {
		fp := f.FilePath
		if r, rerr := filepath.Rel(e.sourceRoot, fp); rerr == nil {
			fp = r
		}
		dbF = append(dbF, agentdb.ScannerFinding{
			RunID: runID, Auditor: f.Auditor, Severity: f.Severity,
			FilePath: fp, LineNumber: f.LineNumber, Message: f.Message,
			Snippet: f.Snippet, Remediation: f.Remediation,
		})
		switch f.Severity {
		case scanner.SevCritical:
			crit++
		case scanner.SevHigh:
			high++
		}
	}
	_ = store.InsertScannerFindings(runID, dbF)
	status := "pass"
	if crit > 0 {
		status = "fail"
	}
	_ = store.FinishScannerRun(runID, len(dbF), crit, status)
	if crit+high > 0 {
		sev := "warning"
		if crit > 0 {
			sev = "critical"
		}
		detail, _ := json.Marshal(map[string]any{"scope": "repo-baseline", "critical": crit, "high": high, "total": len(dbF), "run_id": runID})
		_, _ = store.AppendAudit(agentdb.AuditEntry{EventType: "scanner_finding", Severity: sev, Actor: "codescan", DetailJSON: string(detail)})
	}
	log.Printf("[codescan] baseline scan done — %d findings (crit=%d high=%d) di %d file", len(dbF), crit, high, res.FilesScanned)
}

// watchDirs — kumpulin semua dir yang di-watch: source repo (skip noise) +
// /shared/<id>/tools/ tiap agent.
func (e *Engine) watchDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	if e.sourceRoot != "" {
		_ = filepath.Walk(e.sourceRoot, func(path string, info os.FileInfo, werr error) error {
			if werr != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if path != e.sourceRoot && (skipDirs[name] || strings.HasPrefix(name, ".")) {
					return filepath.SkipDir
				}
				add(path)
			}
			return nil
		})
	}
	if e.enum != nil && e.sharedDir != nil {
		for _, id := range e.enum() {
			if sd, err := e.sharedDir(id); err == nil && sd != "" {
				add(filepath.Join(sd, "tools"))
			}
		}
	}
	return dirs
}

func (e *Engine) loop(ctx context.Context) {
	defer e.watcher.Close()
	e.timer = time.NewTimer(time.Hour)
	e.timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-e.watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if !scanExt[strings.ToLower(filepath.Ext(ev.Name))] {
				continue
			}
			e.mu.Lock()
			if len(e.pending) < maxBatch {
				e.pending[ev.Name] = true
			}
			e.mu.Unlock()
			e.timer.Reset(debounceWait)
		case werr, ok := <-e.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[codescan] watch error: %v", werr)
		case <-e.timer.C:
			e.flush(ctx)
		}
	}
}

func (e *Engine) flush(ctx context.Context) {
	e.mu.Lock()
	paths := make([]string, 0, len(e.pending))
	for p := range e.pending {
		paths = append(paths, p)
	}
	e.pending = map[string]bool{}
	e.mu.Unlock()
	if len(paths) > 0 {
		e.scanPaths(ctx, paths, "auto:filechange")
	}
}

// scanPaths — scan tiap file, persist run+findings ke store primaryAgent,
// audit + notify kalau ada critical/high. Return (totalFindings, totalAlert).
func (e *Engine) scanPaths(ctx context.Context, paths []string, scanType string) (int, int) {
	store, err := e.opener(primaryAgent)
	if err != nil {
		log.Printf("[codescan] open store %q: %v", primaryAgent, err)
		return 0, 0
	}
	defer store.Close()

	totalFindings, totalAlert := 0, 0
	for _, p := range paths {
		info, serr := os.Stat(p)
		if serr != nil || info.IsDir() {
			continue
		}
		rel := p
		if e.sourceRoot != "" {
			if r, rerr := filepath.Rel(e.sourceRoot, p); rerr == nil {
				rel = r
			}
		}
		runID, ierr := store.InsertScannerRun(scanType, rel)
		if ierr != nil {
			continue
		}
		res, rerr := scanner.Run(scanner.RunOptions{Target: p})
		if rerr != nil {
			_ = store.FinishScannerRun(runID, 0, 0, "fail")
			continue
		}
		dbF := make([]agentdb.ScannerFinding, 0, len(res.Findings))
		crit, high := 0, 0
		for _, f := range res.Findings {
			dbF = append(dbF, agentdb.ScannerFinding{
				RunID: runID, Auditor: f.Auditor, Severity: f.Severity,
				FilePath: rel, LineNumber: f.LineNumber, Message: f.Message,
				Snippet: f.Snippet, Remediation: f.Remediation,
			})
			switch f.Severity {
			case scanner.SevCritical:
				crit++
			case scanner.SevHigh:
				high++
			}
		}
		_ = store.InsertScannerFindings(runID, dbF)
		status := "pass"
		if crit > 0 {
			status = "fail"
		}
		_ = store.FinishScannerRun(runID, len(dbF), crit, status)
		totalFindings += len(dbF)

		if crit+high > 0 {
			totalAlert += crit + high
			sev := "warning"
			if crit > 0 {
				sev = "critical"
			}
			detail, _ := json.Marshal(map[string]any{
				"file": rel, "critical": crit, "high": high,
				"total": len(dbF), "run_id": runID, "scan_type": scanType,
			})
			_, _ = store.AppendAudit(agentdb.AuditEntry{
				EventType: "scanner_finding", Severity: sev,
				Actor: "codescan", DetailJSON: string(detail),
			})
			if e.notifier != nil {
				title := "🛡️ Scanner: temuan di " + rel
				body := fmt.Sprintf("File: %s\nCritical: %d · High: %d · Total: %d\nTop: %s",
					rel, crit, high, len(dbF), topFinding(res.Findings))
				_ = e.notifier(ctx, title, body)
			}
			log.Printf("[codescan] %s → critical=%d high=%d total=%d (run %d)", rel, crit, high, len(dbF), runID)
		}
	}
	return totalFindings, totalAlert
}

// topFinding — temuan paling parah buat ringkasan notif.
func topFinding(fs []scanner.Finding) string {
	for _, f := range fs {
		if f.Severity == scanner.SevCritical {
			return f.Auditor + ": " + f.Message
		}
	}
	for _, f := range fs {
		if f.Severity == scanner.SevHigh {
			return f.Auditor + ": " + f.Message
		}
	}
	if len(fs) > 0 {
		return fs[0].Auditor + ": " + fs[0].Message
	}
	return ""
}
