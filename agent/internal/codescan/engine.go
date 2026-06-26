// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

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

type AgentEnumerator func() []string
type StoreOpener func(agentID string) (*agentdb.Store, error)
type SharedDirFunc func(agentID string) (string, error)

type Notifier func(ctx context.Context, title, body string) error

const primaryAgent = "mr-flow"

const (
	debounceWait = 1500 * time.Millisecond
	maxBatch     = 50
)

var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "referensifile": true,
	"web": true, "bin": true, ".scratch": true, "sdk": true, "__pycache__": true,
	"workspace": true,
}

var scanExt = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".sh": true,
	".rb": true, ".rs": true, ".php": true, ".java": true, ".c": true, ".cpp": true,
}

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

func New(enum AgentEnumerator, opener StoreOpener, sharedDir SharedDirFunc, notifier Notifier, sourceRoot string) *Engine {
	return &Engine{
		enum: enum, opener: opener, sharedDir: sharedDir, notifier: notifier,
		sourceRoot: sourceRoot, pending: map[string]bool{},
	}
}

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

	go e.scanRepoBaseline(ctx)
}

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

	res.Findings = append(res.Findings, scanner.ToolScan(e.sourceRoot)...)
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

			if !scanExt[strings.ToLower(filepath.Ext(ev.Name))] && !scanner.IsDepManifest(ev.Name) {
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
	if len(paths) > 0 && scannerAutoscanEnabled() {
		e.scanPaths(ctx, paths, "auto:filechange")
	}
}

func scannerAutoscanEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_SCANNER_AUTOSCAN"))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

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

		if scanner.IsDepManifest(p) {
			res.Findings = append(res.Findings, scanner.ToolScan(p)...)
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
