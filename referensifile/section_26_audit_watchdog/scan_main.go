// flowork-scan — I-C.3 FreelanceScanner CLI + daemon (FASE 12 Earner Type A).
//
// Scans WeWorkRemotely RSS + RemoteOK JSON for remote jobs matching the
// configured skill set. Surfaces top matches to Ayah via Telegram push
// notification for manual review and submission.
//
// Two modes:
//   - one-shot (default): scan + push + exit. Cocok untuk cron eksternal.
//   - daemon (--daemon):  scheduler loop internal, tick every --interval jam.
//     Silent-hours aware (skip push saat 00:00-05:00 WIB).
//
// LEGAL GATE: requires Ayah approval in FASE12_LEGAL_GATE.md before
// running against live feeds. Use --dry-run for local testing.
//
// Usage:
//
//	flowork-scan                   # scan + push top matches ke Telegram (one-shot)
//	flowork-scan --min-score 60    # lower threshold (more results)
//	flowork-scan --dry-run         # fetch + print, no Telegram push
//	flowork-scan --bounties        # also scan GitHub bounties
//	flowork-scan --top 10          # show top 10 (default: 5)
//	flowork-scan --daemon          # loop scheduler mode (default interval 6h)
//	flowork-scan --daemon --interval 4   # tick tiap 4 jam
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	earners "github.com/teetah2402/flowork/internal/earners/deprecated"

	"github.com/teetah2402/flowork/internal/bootannounce"
	"github.com/teetah2402/flowork/internal/config"

	"github.com/teetah2402/flowork/internal/silentmode"

	_ "modernc.org/sqlite"
)

const (
	defaultIntervalHours = 6
	daemonStartupDelay   = 30 * time.Second
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "flowork-scan:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// ── Parse flags ────────────────────────────────────────────────
	minScore := 75
	top := 5
	dryRun := false
	includeBounties := false
	daemon := false
	intervalHours := 0 // 0 = resolve from env/default

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run", "-n":
			dryRun = true
		case "--bounties", "-b":
			includeBounties = true
		case "--daemon", "-d":
			daemon = true
		case "--interval":
			if i+1 >= len(args) {
				return fmt.Errorf("--interval requires a value (hours)")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 1 {
				return fmt.Errorf("--interval must be >= 1 (hours)")
			}
			intervalHours = v
		case "--min-score":
			if i+1 >= len(args) {
				return fmt.Errorf("--min-score requires a value")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 0 || v > 100 {
				return fmt.Errorf("--min-score must be 0-100")
			}
			minScore = v
		case "--top":
			if i+1 >= len(args) {
				return fmt.Errorf("--top requires a value")
			}
			i++
			v, err := strconv.Atoi(args[i])
			if err != nil || v < 1 {
				return fmt.Errorf("--top must be >= 1")
			}
			top = v
		case "--help", "-h":
			printHelp()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	// ── Daemon mode (rc78) — takeover from peer opus-2 per Ayah ──────
	if daemon {
		return runDaemon(daemonConfig{
			MinScore:        minScore,
			Top:             top,
			IncludeBounties: includeBounties,
			IntervalHours:   intervalHours,
		})
	}

	fmt.Fprintf(os.Stderr, "flowork-scan: scanning job feeds (min-score=%d, top=%d, dry-run=%v)\n",
		minScore, top, dryRun)

	// ── Fetch opportunities ────────────────────────────────────────
	var all []earners.Opportunity

	freelanceOpps, err := earners.Scan()
	if err != nil {
		fmt.Fprintf(os.Stderr, "flowork-scan: freelance scan error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "flowork-scan: freelance — %d opportunities fetched\n", len(freelanceOpps))
		all = append(all, freelanceOpps...)
	}

	if includeBounties {
		bountyOpps, err := earners.ScanBounties()
		if err != nil {
			fmt.Fprintf(os.Stderr, "flowork-scan: bounty scan error: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "flowork-scan: bounties — %d opportunities fetched\n", len(bountyOpps))
			all = append(all, bountyOpps...)
		}
	}

	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "flowork-scan: no opportunities found from any source")
		return nil
	}

	// ── Format report ──────────────────────────────────────────────
	topMatches := earners.TopMatches(all, minScore, top)
	report := earners.FormatOpportunitiesReport(all, minScore)

	fmt.Println(report)
	fmt.Fprintf(os.Stderr, "flowork-scan: %d matches above score %d (from %d total)\n",
		len(topMatches), minScore, len(all))

	if dryRun {
		fmt.Fprintln(os.Stderr, "flowork-scan: --dry-run — skipping Telegram push")
		return nil
	}

	// Kill switch — settings.earners_enabled=false → skip push. Ayah 2026-04-24
	// "kita sudah ngak akan pake ini" (freelance scan disabled).
	if !earnerScanEnabled() {
		fmt.Fprintln(os.Stderr, "flowork-scan: earners_enabled=false di settings — skipping Telegram push")
		return nil
	}

	// ── Push to Telegram ───────────────────────────────────────────
	if err := earners.NotifyAyahTelegram(all, minScore); err != nil {
		fmt.Fprintf(os.Stderr, "flowork-scan: notify error: %v\n", err)
	}

	return nil
}

// earnerScanEnabled reads settings.earners_enabled dari flowork-settings.sqlite.
// Returns true (allow push) kalau flag unset atau true. False = disabled.
// Fail-closed: kalau DB error, return true (default on) supaya kalau DB rusak
// sementara push tetep jalan. Ayah bisa delete binary kalau mau permanent kill.
func earnerScanEnabled() bool {
	workspace, err := os.Getwd()
	if err != nil {
		return true
	}
	dbPath := filepath.Join(workspace, "brain", "flowork-settings.sqlite")
	database, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(3000)")
	if err != nil {
		return true
	}
	defer database.Close()
	var val string
	err = database.QueryRow("SELECT value FROM settings WHERE key='earners_enabled'").Scan(&val)
	if err != nil {
		return true // flag not set → default enabled
	}
	return strings.ToLower(strings.TrimSpace(val)) != "false"
}

// daemonConfig holds resolved settings for the scheduler loop.
type daemonConfig struct {
	MinScore        int
	Top             int
	IncludeBounties bool
	IntervalHours   int
}

// runDaemon menjalankan scheduler loop yang scan setiap IntervalHours.
// Silent-hours aware: scan tetap fetch tapi skip Telegram push saat quiet.
// Graceful shutdown pada SIGINT/SIGTERM.
//
// Per Ayah 2026-04-18: takeover scope P7 dari peer opus-2 yang sibuk.
func runDaemon(cfg daemonConfig) error {
	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	config.LoadDotEnv(workspace)

	// Resolve interval: flag → env → default.
	hours := cfg.IntervalHours
	if hours == 0 {
		if v := os.Getenv("FLOWORK_SCAN_INTERVAL_HOURS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				hours = n
			}
		}
	}
	if hours == 0 {
		hours = defaultIntervalHours
	}

	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Printf("flowork-scan daemon: interval=%dh min-score=%d top=%d bounties=%v workspace=%s",
		hours, cfg.MinScore, cfg.Top, cfg.IncludeBounties, workspace)

	// Boot announce → dashboard /api/agents tandai "scan" ALIVE.
	go bootannounce.Boot("scan", workspace,
		fmt.Sprintf("flowork-scan daemon (interval %dh, min-score %d)", hours, cfg.MinScore))

	// Graceful shutdown channel.
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[RECOVER] scan signal handler panic: %v", r)
				close(done)
			}
		}()
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		close(done)
	}()

	// Startup delay supaya tim binary lain sempat boot lebih dulu.
	log.Printf("flowork-scan daemon: startup delay %v sebelum scan pertama", daemonStartupDelay)
	select {
	case <-time.After(daemonStartupDelay):
	case <-done:
		log.Println("flowork-scan daemon: shutdown sebelum first-scan")
		return nil
	}

	runDaemonCycle(cfg)

	ticker := time.NewTicker(time.Duration(hours) * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			log.Println("flowork-scan daemon: shutting down...")
			return nil
		case <-ticker.C:
			runDaemonCycle(cfg)
		}
	}
}

// runDaemonCycle eksekusi satu putaran scan + notify. Silent-hours skip push.
func runDaemonCycle(cfg daemonConfig) {
	now := time.Now()
	log.Printf("flowork-scan daemon: cycle begin")

	var all []earners.Opportunity
	freelance, err := earners.Scan()
	if err != nil {
		log.Printf("flowork-scan daemon: Scan error: %v", err)
	}
	log.Printf("flowork-scan daemon: freelance %d opps", len(freelance))
	all = append(all, freelance...)

	if cfg.IncludeBounties {
		bounties, err := earners.ScanBounties()
		if err != nil {
			log.Printf("flowork-scan daemon: ScanBounties error: %v", err)
		}
		log.Printf("flowork-scan daemon: bounty %d opps", len(bounties))
		all = append(all, bounties...)
	}

	if silentmode.IsSilent(now) {
		log.Println("flowork-scan daemon: silent-hours — skip Telegram push (hasil tetap di log)")
		if report := earners.FormatOpportunitiesReport(all, cfg.MinScore); report != "" {
			log.Printf("flowork-scan daemon: silent-report:\n%s", report)
		}
		return
	}

	if err := earners.NotifyAyahTelegram(all, cfg.MinScore); err != nil {
		log.Printf("flowork-scan daemon: NotifyAyahTelegram error: %v", err)
	}
	log.Println("flowork-scan daemon: cycle done")
}

func printHelp() {
	fmt.Println(`flowork-scan — I-C.3 FreelanceScanner (FASE 12 Earner Type A)

LEGAL GATE: Requires Ayah approval in roadmap/FASE12_LEGAL_GATE.md.

Usage:
  flowork-scan [flags]

Flags:
  --min-score N   Minimum match score 0-100 (default: 75)
  --top N         Show top N results (default: 5)
  --bounties      Also scan GitHub bounties
  --dry-run       Print results, skip Telegram push
  --help          Show this help

Environment:
  FLOWORK_FREELANCE_SKILLS   Comma-separated skill keywords (default: go,golang,rust,ai,backend,api,llm,python,devops,cloud)
  FLOWORK_TG_PUSH_PORT       Telegram push port (default: 8900)

Example:
  flowork-scan --dry-run --bounties --min-score 60`)
}
