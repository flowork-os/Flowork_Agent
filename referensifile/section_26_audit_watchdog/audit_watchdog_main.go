// flowork-audit-watchdog — daemon yang jalan otomatis scanner/flowork_*_auditor.go
// tiap kali file .go di internal/ atau cmd/ berubah. Warga AI (aksara/wiraga/dll)
// TIDAK perlu invoke scanner manual lagi — hasilnya ditaruh di docs/bug/ext_*.md
// siap dikonsumsi.
//
// Kenapa polling, bukan fsnotify: cross-platform consistency, no new dep,
// scan workspace ~500 file < 100ms → overhead negligible tiap 10 detik.
//
// Flow:
//  1. Polling loop (default 10s): walk internal/ + cmd/, track file mtime.
//  2. Kalau ada mtime baru → set lastChangeAt = now.
//  3. Debounce (default 15s idle): kalau belum running + idle cukup → trigger
//     scanner suite.
//  4. Concurrent run (default 4 parallel): exec `go run scanner/flowork_*_auditor.go`
//     per file. Output (docs/bug/ + state/scanner-reports/) ditulis auditor sendiri.
//  5. Heartbeat tiap 30s ke state/audit-watchdog/heartbeat.json supaya warga lain
//     bisa check apakah watchdog ALIVE.
//
// Register ke supervisor: nanti via sistem supervisor (cmd/flowork-mesh atau
// similar). Untuk sekarang: jalanin manual via `build/flowork-audit-watchdog.exe`.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	workspaceFlag = flag.String("workspace", "", "Flowork workspace root (default: cwd)")
	pollInterval  = flag.Duration("poll", 10*time.Second, "Interval polling file mtime")
	debounce      = flag.Duration("debounce", 15*time.Second, "Idle period setelah last change sebelum trigger")
	concurrency   = flag.Int("concurrency", 4, "Max parallel auditor runs")
	heartbeatFreq = flag.Duration("heartbeat", 30*time.Second, "Heartbeat write frequency")
	runOnStart    = flag.Bool("run-on-start", true, "Trigger full scan sekali di awal startup (baseline)")
	auditorGlob   = flag.String("auditor-glob", "scanner/flowork_*_auditor.go", "Glob pattern untuk auditor scripts")
)

func main() {
	flag.Parse()

	ws := strings.TrimSpace(*workspaceFlag)
	if ws == "" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatalf("[audit-watchdog] getwd: %v", err)
		}
		ws = cwd
	}
	log.Printf("[audit-watchdog] start workspace=%s poll=%s debounce=%s concurrency=%d",
		ws, *pollInterval, *debounce, *concurrency)

	stateDir := filepath.Join(ws, "state", "audit-watchdog")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		log.Fatalf("[audit-watchdog] mkdir stateDir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("[audit-watchdog] shutdown signal — waiting current scan drain")
		cancel()
	}()

	var (
		mu           sync.Mutex
		knownMtimes  = map[string]time.Time{}
		lastChangeAt time.Time
		running      atomic.Bool
		lastRunEnd   time.Time
	)

	// Heartbeat goroutine
	go func() {
		t := time.NewTicker(*heartbeatFreq)
		defer t.Stop()
		writeHeartbeat(stateDir, running.Load(), lastRunEnd)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				writeHeartbeat(stateDir, running.Load(), lastRunEnd)
			}
		}
	}()

	// Optional baseline run — supaya docs/bug/ langsung ke-populate di first boot.
	if *runOnStart {
		log.Println("[audit-watchdog] baseline run (startup)")
		running.Store(true)
		go func() {
			defer running.Store(false)
			runAuditors(ctx, ws, *auditorGlob, *concurrency)
			lastRunEnd = time.Now()
		}()
	}

	// Initial mtime snapshot (tanpa trigger — cuma populate known).
	_ = scanSources(ws, knownMtimes, &mu, true)

	poll := time.NewTicker(*pollInterval)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[audit-watchdog] exit")
			return
		case <-poll.C:
			if scanSources(ws, knownMtimes, &mu, false) {
				mu.Lock()
				lastChangeAt = time.Now()
				mu.Unlock()
				log.Printf("[audit-watchdog] change detected, debounce %s", *debounce)
			}

			mu.Lock()
			lca := lastChangeAt
			mu.Unlock()

			if !lca.IsZero() && time.Since(lca) >= *debounce && !running.Load() {
				mu.Lock()
				lastChangeAt = time.Time{}
				mu.Unlock()
				running.Store(true)
				go func() {
					defer running.Store(false)
					runAuditors(ctx, ws, *auditorGlob, *concurrency)
					lastRunEnd = time.Now()
				}()
			}
		}
	}
}

// scanSources walk internal/ + cmd/, track mtime. Return true kalau ada file
// baru atau mtime lebih baru dari last seen. suppressFirst = true untuk seed
// awal (populate known tanpa claim change).
func scanSources(ws string, known map[string]time.Time, mu *sync.Mutex, suppressFirst bool) bool {
	roots := []string{"internal", "cmd"}
	anyNew := false
	for _, rel := range roots {
		root := filepath.Join(ws, rel)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				n := d.Name()
				if n == ".git" || n == "node_modules" || n == "vendor" || n == "build" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			mu.Lock()
			prev, seen := known[path]
			if !seen || info.ModTime().After(prev) {
				known[path] = info.ModTime()
				if seen || !suppressFirst {
					anyNew = true
				}
			}
			mu.Unlock()
			return nil
		})
	}
	return anyNew
}

func runAuditors(ctx context.Context, ws, glob string, concurrency int) {
	matches, err := filepath.Glob(filepath.Join(ws, glob))
	if err != nil {
		log.Printf("[audit-watchdog] glob err: %v", err)
		return
	}
	if len(matches) == 0 {
		log.Printf("[audit-watchdog] no auditors matched (glob=%s)", glob)
		return
	}
	log.Printf("[audit-watchdog] run %d auditors (concurrency=%d)", len(matches), concurrency)
	start := time.Now()

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var failed atomic.Int32
	var timeouts atomic.Int32
	for _, auditor := range matches {
		select {
		case <-ctx.Done():
			log.Println("[audit-watchdog] ctx canceled mid-run — waiting goroutines drain")
			goto WAIT
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p string) {
			defer wg.Done()
			defer func() { <-sem }()
			cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(cmdCtx, "go", "run", p)
			cmd.Dir = ws
			out, err := cmd.CombinedOutput()
			if err != nil {
				if cmdCtx.Err() == context.DeadlineExceeded {
					timeouts.Add(1)
					log.Printf("[audit-watchdog] TIMEOUT %s (3m)", filepath.Base(p))
				} else {
					failed.Add(1)
					log.Printf("[audit-watchdog] FAIL %s: %v snippet=%q", filepath.Base(p), err, snippet(out))
				}
			}
		}(auditor)
	}
WAIT:
	wg.Wait()
	log.Printf("[audit-watchdog] done %s (failed=%d timeouts=%d total=%d)",
		time.Since(start).Round(time.Millisecond), failed.Load(), timeouts.Load(), len(matches))
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func writeHeartbeat(stateDir string, running bool, lastRunEnd time.Time) {
	beat := map[string]any{
		"ts":           time.Now().UTC().Format(time.RFC3339),
		"running":      running,
		"pid":          os.Getpid(),
		"last_run_end": lastRunEnd.UTC().Format(time.RFC3339),
	}
	if lastRunEnd.IsZero() {
		beat["last_run_end"] = ""
	}
	data, _ := json.MarshalIndent(beat, "", "  ")
	_ = os.WriteFile(filepath.Join(stateDir, "heartbeat.json"), data, 0o644)
}
