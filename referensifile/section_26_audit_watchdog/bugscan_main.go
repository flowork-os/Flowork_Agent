// flowork-bugscan — P3-1 autonomous periodic code auditor daemon.
//
// Beda dengan flowork-bughunter (LLM one-shot via Gemini Pro), daemon ini
// scan-based + periodic — tidak consume LLM budget, running 24/7 sebagai
// warga real (heartbeat ke state/status/bughunter.json supaya Mood Radar
// detect ALIVE).
//
// Loop: tiap FLOWORK_BUGSCAN_INTERVAL (default 6h) do:
//  1. Grep TODO/XXX/FIXME/HACK di .go files (skip _test + vendor)
//  2. Merge SGVP scanner reports dari state/scanner-reports/*.md
//  3. Register bug baru ke registry (dedup via stableID(file,line,title))
//  4. Heartbeat update ke status.Write supaya GUI real-time
//  5. Telegram alert owner kalau ada critical/high baru (opsional)
//
// Env:
//
//	FLOWORK_BUGSCAN_INTERVAL  — jam interval (default 6)
//	FLOWORK_BUGSCAN_NOTIFY    — "1" = telegram alert kalau ada critical/high
//	FLOWORK_BUGSCAN_DRY_RUN   — "1" = scan + log, don't register
//	FLOWORK_BUGSCAN_AGENT     — heartbeat agent name (default "bughunter")
//
// Usage:
//
//	flowork-bugscan
//	flowork-bugscan --dry-run
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/teetah2402/flowork/internal/bugregistry"
	"github.com/teetah2402/flowork/internal/config"
	"github.com/teetah2402/flowork/internal/notify"
	"github.com/teetah2402/flowork/internal/status"
)

const version = "0.1.0"

// Pattern TODO/XXX/FIXME/HACK di source comment.
var todoPattern = regexp.MustCompile(`(?i)(?:^|\s|//|/\*|#)\s*(TODO|XXX|FIXME|HACK)[\s(:]+(.+?)(?:\*/|$)`)

func main() {
	var dryRunFlag bool
	var onceFlag bool
	flag.BoolVar(&dryRunFlag, "dry-run", false, "Scan tanpa register ke bug registry")
	flag.BoolVar(&onceFlag, "once", false, "Single scan cycle then exit (untuk dipanggil dari scheduler)")
	flag.Parse()

	cwd, _ := os.Getwd()
	workspace, _ := filepath.Abs(cwd)
	config.LoadDotEnv(workspace)
	status.AutoHeartbeat("bughunter", 60*time.Second)

	intervalHours := 6
	if v := os.Getenv("FLOWORK_BUGSCAN_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			intervalHours = n
		}
	}
	dryRun := dryRunFlag || os.Getenv("FLOWORK_BUGSCAN_DRY_RUN") == "1"
	notifyOwner := os.Getenv("FLOWORK_BUGSCAN_NOTIFY") == "1"
	agentName := strings.TrimSpace(os.Getenv("FLOWORK_BUGSCAN_AGENT"))
	if agentName == "" {
		agentName = "bughunter"
	}

	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Printf("[BUGSCAN] v%s starting | interval=%dh dry_run=%v notify=%v agent=%s once=%v workspace=%s",
		version, intervalHours, dryRun, notifyOwner, agentName, onceFlag, workspace)

	if onceFlag {
		runCycle(workspace, agentName, dryRun, notifyOwner, intervalHours)
		log.Printf("[BUGSCAN] --once mode complete, exiting")
		return
	}

	heartbeat(agentName, status.StateIdle, "bootup — first scan in 10s", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; cancel() }()

	// First scan 10s after boot, subsequent tiap interval
	firstTimer := time.NewTimer(10 * time.Second)
	defer firstTimer.Stop()
	ticker := time.NewTicker(time.Duration(intervalHours) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			heartbeat(agentName, status.StateOffline, "shutdown by signal", "")
			log.Printf("[BUGSCAN] exit")
			return
		case <-firstTimer.C:
			runCycle(workspace, agentName, dryRun, notifyOwner, intervalHours)
		case <-ticker.C:
			runCycle(workspace, agentName, dryRun, notifyOwner, intervalHours)
		}
	}
}

func runCycle(workspace, agentName string, dryRun, notifyOwner bool, intervalHours int) {
	cycleStart := time.Now()
	heartbeat(agentName, status.StateThinking, "scanning codebase", "")

	// Phase 1: grep TODO/XXX/FIXME/HACK di .go files
	todos := scanTodoMarkers(workspace)
	log.Printf("[BUGSCAN] phase1: found %d TODO/XXX/FIXME markers", len(todos))

	// Phase 2: merge SGVP scanner reports
	sgvp := scanSGVPReports(workspace)
	log.Printf("[BUGSCAN] phase2: found %d SGVP findings", len(sgvp))
	if dryRun {
		max := len(sgvp)
		cap := 20
		if os.Getenv("FLOWORK_BUGSCAN_DRY_VERBOSE") == "1" {
			cap = max
		}
		if max > cap {
			max = cap
		}
		for i := 0; i < max; i++ {
			b := sgvp[i]
			log.Printf("[BUGSCAN]   #%d [%s] %s @ %s:%d (%s)", i+1, b.Severity, b.Title, b.File, b.Line, b.Source)
		}
		if len(sgvp) > max {
			log.Printf("[BUGSCAN]   ... +%d more (set FLOWORK_BUGSCAN_DRY_VERBOSE=1 to see all)", len(sgvp)-max)
		}
	}

	// Phase 3: register (dedup auto via stableID)
	registered, reopened := 0, 0
	newCritHigh := []bugregistry.Bug{}
	for _, b := range append(todos, sgvp...) {
		if dryRun {
			continue
		}
		id, wasNew, err := bugregistry.Register(workspace, b)
		if err != nil {
			log.Printf("[BUGSCAN] register fail %q: %v", b.Title, err)
			continue
		}
		if wasNew {
			registered++
			if b.Severity == bugregistry.SeverityCritical || b.Severity == bugregistry.SeverityHigh {
				b.ID = id
				newCritHigh = append(newCritHigh, b)
			}
		} else {
			reopened++
		}
	}

	elapsed := time.Since(cycleStart).Round(time.Second)
	summary := fmt.Sprintf("scan: todo=%d sgvp=%d new=%d reopened=%d elapsed=%s",
		len(todos), len(sgvp), registered, reopened, elapsed)
	log.Printf("[BUGSCAN] %s", summary)
	next := time.Now().Add(time.Duration(intervalHours) * time.Hour).Format("15:04 UTC")
	heartbeat(agentName, status.StateIdle,
		fmt.Sprintf("scan done — next %s", next),
		summary)

	// Phase 4: alert owner kalau ada critical/high baru
	if notifyOwner && len(newCritHigh) > 0 {
		msg := fmt.Sprintf("🐛 *Bughunter found %d new critical/high bug(s)*\n\n", len(newCritHigh))
		shown := 0
		for _, b := range newCritHigh {
			if shown >= 5 {
				msg += fmt.Sprintf("\n…+%d more (check dashboard bugs tab)", len(newCritHigh)-shown)
				break
			}
			loc := ""
			if b.File != "" {
				loc = fmt.Sprintf(" `%s:%d`", b.File, b.Line)
			}
			title := b.Title
			if len(title) > 80 {
				title = title[:77] + "..."
			}
			msg += fmt.Sprintf("• [%s]%s %s\n", b.Severity, loc, title)
			shown++
		}
		notify.AlertOwnerFireForget(msg)
	}
}

// ─── Phase 1: TODO/XXX/FIXME grep ─────────────────────────────────────

func scanTodoMarkers(root string) []bugregistry.Bug {
	var out []bugregistry.Bug
	skipDirs := map[string]bool{
		"node_modules": true, "vendor": true, ".git": true,
		"state": true, "bin": true, "_sgvp": true,
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		// 2026-05-05 fix (Ayah audit Tab Sistem Imun Bug Tracker valid?):
		// skip self-reference. internal/bughunter/* dan scanner/* mengandung
		// TODO sebagai pattern/regex spec/example — bukan TODO actionable.
		// Match logic shouldSkipForTodoScan() di internal/bughunter/hunter.go.
		rel, _ := filepath.Rel(root, path)
		slash := filepath.ToSlash(rel)
		if strings.HasPrefix(slash, "internal/bughunter/") ||
			strings.HasPrefix(slash, "scanner/") ||
			strings.HasPrefix(slash, "internal/scanner/") ||
			strings.HasPrefix(slash, "cmd/flowork-bugscan/") ||
			strings.HasPrefix(slash, "cmd/flowork-bughunter/") {
			return nil
		}
		f, ferr := os.Open(path)
		if ferr != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			m := todoPattern.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			marker := strings.ToUpper(m[1])
			title := strings.TrimSpace(m[2])
			if len(title) > 140 {
				title = title[:137] + "..."
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			sev := inferSeverity(marker, rel)
			out = append(out, bugregistry.Bug{
				Title:       fmt.Sprintf("[%s] %s", marker, title),
				File:        rel,
				Line:        lineNum,
				Severity:    sev,
				Source:      "bugscan/todo-grep",
				Description: line,
				Tags:        []string{"todo-marker", strings.ToLower(marker), "auto-detected"},
			})
		}
		return nil
	})
	return out
}

// ─── Phase 2: SGVP report merge ───────────────────────────────────────
//
// Universal parser: scanners emit reports in many shapes (bullet list,
// `### [SEV] Title` heading + `**File:** path:line` body, `**[Tag]** File: path:line`,
// etc). Instead of matching one strict format we extract:
//   - severity bracket `[CRITICAL|HIGH|MEDIUM|LOW|INFO]` (default MEDIUM)
//   - title from the header/bullet line
//   - first backticked `path:line` (or `path`) inside the block
// A "block" is delimited by `---`, `### `, or single-line bullet starts.

// Reports that produce more noise than signal (subjective roasts /
// ADR-012 rename batches that aren't real runtime bugs / scanner over-flags
// every Go package-level var as "Global State Poisoning").
var sgvpSkipReports = map[string]bool{
	"antigravity_roast_session.md": true, // subjective cyclomatic roast, low signal
	"omni_arsenal_report.md":       true, // 247 ADR-012 rename items, not bugs
	"pandoras_box_audit.md":        true, // flags every package-level var as poisoning (FP > 95%)
}

// Bracketed severity like [CRITICAL] [HIGH] [MEDIUM] [LOW] [INFO].
var sevBracketRE = regexp.MustCompile(`(?i)\[(CRITICAL|HIGH|MEDIUM|LOW|INFO)\]`)

// Category-tag → default severity. Findings without an explicit severity but
// emitted with a known category bracket (e.g. `[Subprocess Leak]`,
// `[Path Traversal Risk]`) get this fallback severity.
var categoryDefaultSev = map[string]bugregistry.Severity{
	"path traversal risk":     bugregistry.SeverityHigh,
	"subprocess leak":         bugregistry.SeverityMedium,
	"fd leak":                 bugregistry.SeverityMedium,
	"goroutine leak":          bugregistry.SeverityMedium,
	"http hanging":            bugregistry.SeverityMedium,
	"global state poisoning":  bugregistry.SeverityHigh,
	"toctou":                  bugregistry.SeverityHigh,
	"injection":               bugregistry.SeverityHigh,
	"ssrf":                    bugregistry.SeverityHigh,
	"data poisoning":          bugregistry.SeverityHigh,
	"unsafe deserialization":  bugregistry.SeverityHigh,
	"crypto timing attack":    bugregistry.SeverityHigh,
	"hardcoded secret":        bugregistry.SeverityHigh,
	"public network exposure": bugregistry.SeverityCritical,
}

// Bracketed category extractor: `[Subprocess Leak]`, `[Path Traversal Risk]`.
// Used as fallback when no [CRIT/HIGH/MED/LOW/INFO] is present.
var categoryBracketRE = regexp.MustCompile(`\[([A-Za-z][A-Za-z\s]+?)\]`)

// Backticked path:line — accepts both forward + back slashes.
// Examples: `internal\tools\permissions.go:541`, `cmd/flowork-gui/main.go:446`.
var pathLineRE = regexp.MustCompile("`([A-Za-z_0-9./\\\\-]+\\.(?:go|md|py|sh|js|ts|json|sql|yml|yaml|toml|html|css)):(\\d+)`")

// Backticked path without line number — fallback when scanner only emits file.
var pathOnlyRE = regexp.MustCompile("`([A-Za-z_0-9./\\\\-]+\\.(?:go|md|py|sh|js|ts|json|sql|yml|yaml|toml|html|css))`")

// Heading start: `### ` (with optional emoji) or `## ` (level-2 finding heading).
var blockStartRE = regexp.MustCompile(`^#{2,4}\s+`)

// Single-line bullet: `- **[SEV] Title** — ` ... `path:line` ...
var bulletFindingRE = regexp.MustCompile("^-\\s+\\*\\*\\[([A-Z]+)\\]\\s+(.+?)\\*\\*")

// Category-tag standalone: `**[Subprocess Leak]** File: \`path:line\``
// Used by memory_leak_audit which doesn't wrap each finding in its own heading.
var tagStandaloneRE = regexp.MustCompile("^\\*\\*\\[([A-Za-z][A-Za-z\\s]+?)\\]\\*\\*\\s+File:\\s+`([^`]+)`")

// Title extractor for headings: strip leading emoji + leading `[SEV]` if present.
var leadingEmojiRE = regexp.MustCompile(`^[^A-Za-z\[]+`)

func scanSGVPReports(workspace string) []bugregistry.Bug {
	var out []bugregistry.Bug
	dir := filepath.Join(workspace, "state", "scanner-reports")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if sgvpSkipReports[e.Name()] {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parseSGVPReport(string(data), e.Name(), &out)
	}
	return out
}

// parseSGVPReport splits the markdown into blocks and tries to recognise a
// finding inside each block. Falls back to single-line bullet form for the
// `ext_*_report.md` scanners.
func parseSGVPReport(content, reportName string, out *[]bugregistry.Bug) {
	scannerTag := strings.TrimSuffix(reportName, ".md")
	scannerTag = strings.TrimSuffix(strings.TrimPrefix(scannerTag, "ext_"), "_report")

	lines := strings.Split(content, "\n")
	// In-report dedup so a finding picked up by both bullet+heading pass
	// is registered once. Key = file:line.
	seen := make(map[string]bool)
	emit := func(b *bugregistry.Bug) {
		if b == nil {
			return
		}
		key := b.File + ":" + strconv.Itoa(b.Line)
		if seen[key] {
			return
		}
		seen[key] = true
		*out = append(*out, *b)
	}

	// First pass: bullet form (`- **[SEV] Title** — ...path:line...`).
	for _, line := range lines {
		m := bulletFindingRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		bug := buildBugFromHeader(m[1], m[2], line, scannerTag)
		emit(bug)
	}

	// Second-A pass: category-tag standalone lines (memory_leak_audit form).
	for i, line := range lines {
		m := tagStandaloneRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		category := strings.TrimSpace(m[1])
		key := strings.ToLower(category)
		sev, ok := categoryDefaultSev[key]
		if !ok {
			sev = bugregistry.SeverityMedium
		}
		// Pull the next line as description if it's a quote line.
		desc := line
		if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), ">") {
			desc = line + "\n" + lines[i+1]
		}
		title := category
		bug := buildBugFromHeader(string(sev), title, desc+"\n`"+m[2]+"`", scannerTag)
		emit(bug)
	}

	// Second pass: heading-block form. Slice into blocks by heading start.
	var blockStart = -1
	flush := func(end int) {
		if blockStart < 0 || blockStart >= end {
			return
		}
		block := lines[blockStart:end]
		emit(parseHeadingBlock(block, scannerTag))
	}
	for i, line := range lines {
		if blockStartRE.MatchString(line) {
			flush(i)
			blockStart = i
		}
	}
	flush(len(lines))
}

// parseHeadingBlock extracts a single Bug from a `### …` block. Returns nil if
// the block doesn't carry a recognisable severity + path:line pair.
func parseHeadingBlock(block []string, scannerTag string) *bugregistry.Bug {
	if len(block) == 0 {
		return nil
	}
	header := block[0]
	// Skip top-level document headers like "# 🌌 Analisis…".
	if strings.HasPrefix(header, "# ") {
		return nil
	}
	body := strings.Join(block, "\n")

	// Severity: prefer explicit [CRIT/HIGH/...]. Fallback to category bracket.
	var sevLit string
	if m := sevBracketRE.FindStringSubmatch(body); m != nil {
		sevLit = m[1]
	} else if m := categoryBracketRE.FindStringSubmatch(body); m != nil {
		key := strings.ToLower(strings.TrimSpace(m[1]))
		if mappedSev, ok := categoryDefaultSev[key]; ok {
			sevLit = string(mappedSev)
		}
	}
	if sevLit == "" {
		return nil
	}

	// Title: strip leading `### `, emoji, `[SEV]` / `[Category]`.
	title := strings.TrimSpace(strings.TrimLeft(header, "# "))
	title = leadingEmojiRE.ReplaceAllString(title, "")
	title = sevBracketRE.ReplaceAllString(title, "")
	title = categoryBracketRE.ReplaceAllString(title, "")
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "*— ")
	// If title is just a backticked path (e.g. `cmd\flowork-bugscan\main.go:242`),
	// derive a better title from the body. Detect via: title starts with a backtick
	// AND ≤ 1 word outside backticks.
	if title == "" || isBareBacktickedPath(title) {
		title = deriveTitleFromBody(block, scannerTag)
	}
	if title == "" {
		title = "SGVP Finding (" + scannerTag + ")"
	}
	if len(title) > 140 {
		title = title[:137] + "..."
	}

	bug := buildBugFromHeader(sevLit, title, body, scannerTag)
	return bug
}

// isBareBacktickedPath returns true if `title` is essentially just a backticked
// path (no descriptive text). Used to decide whether to fall back to body-derived
// title.
func isBareBacktickedPath(title string) bool {
	t := strings.TrimSpace(title)
	if !strings.HasPrefix(t, "`") {
		return false
	}
	// Strip the leading-backtick path token.
	rest := t
	if i := strings.Index(t[1:], "`"); i >= 0 {
		rest = strings.TrimSpace(t[1+i+1:])
	} else {
		return true
	}
	// If what remains is empty or just punctuation, the title was bare.
	rest = strings.Trim(rest, "*— :.,;")
	return rest == ""
}

// deriveTitleFromBody scans the block body for a `**Fix:**`, `**Bahaya...:**`,
// `**Analisis:**` or `**Diagnosis:**` line and returns the first sentence.
func deriveTitleFromBody(block []string, scannerTag string) string {
	keys := []string{"**Fix:**", "**Bahaya", "**Analisis", "**Diagnosis", "**Laporan", "**Perlindungan", "**Literal:**", "**Pesan**:", "**Pesan:**"}
	for _, line := range block[1:] {
		for _, k := range keys {
			if strings.Contains(line, k) {
				clean := line
				for _, kk := range keys {
					clean = strings.ReplaceAll(clean, kk, "")
				}
				clean = strings.TrimLeft(clean, "* :—")
				clean = strings.TrimSpace(clean)
				clean = strings.SplitN(clean, ".", 2)[0]
				if len(clean) > 0 && len(clean) < 140 {
					return scannerTagToTitle(scannerTag) + " — " + clean
				}
				return clean
			}
		}
	}
	return scannerTagToTitle(scannerTag) + " finding"
}

func scannerTagToTitle(tag string) string {
	switch tag {
	case "cross_os_path_audit":
		return "Hardcoded path"
	case "antigravity_logic_audit":
		return "Logic anomaly"
	case "elite_taint_audit":
		return "Taint flow"
	case "memory_leak_audit":
		return "Resource leak"
	case "the_fortress_audit":
		return "Fortress violation"
	case "trillion_dollar_audit":
		return "Network exposure"
	case "zeroday_audit":
		return "Zero-day risk"
	case "pandoras_box_audit":
		return "Global state risk"
	}
	return tag
}

// buildBugFromHeader builds a bugregistry.Bug from a severity literal, a title
// and a body containing the file path. Returns nil if no path was found or the
// path is in a skip-list (fixtures, generated dirs).
func buildBugFromHeader(sevLit, title, body, scannerTag string) *bugregistry.Bug {
	sev := bugregistry.SeverityMedium
	switch strings.ToUpper(sevLit) {
	case "CRITICAL":
		sev = bugregistry.SeverityCritical
	case "HIGH":
		sev = bugregistry.SeverityHigh
	case "MEDIUM":
		sev = bugregistry.SeverityMedium
	case "LOW":
		sev = bugregistry.SeverityLow
	case "INFO":
		sev = bugregistry.SeverityInfo
	}

	var file string
	var line int
	if pl := pathLineRE.FindStringSubmatch(body); pl != nil {
		file = filepath.ToSlash(pl[1])
		if n, err := strconv.Atoi(pl[2]); err == nil {
			line = n
		}
	} else if po := pathOnlyRE.FindStringSubmatch(body); po != nil {
		file = filepath.ToSlash(po[1])
	} else {
		return nil
	}
	if isSGVPSkipPath(file) {
		return nil
	}

	title = strings.TrimSpace(title)
	title = strings.Trim(title, "*— ")
	if len(title) > 140 {
		title = title[:137] + "..."
	}

	desc := strings.TrimSpace(body)
	if len(desc) > 500 {
		desc = desc[:497] + "..."
	}

	return &bugregistry.Bug{
		Title:       title,
		File:        file,
		Line:        line,
		Severity:    sev,
		Source:      "bugscan/sgvp:" + scannerTag,
		Description: desc,
		Tags:        []string{"sgvp", "auto-detected", scannerTag},
	}
}

// isSGVPSkipPath excludes scanner self-fixtures and irrelevant artefacts so
// findings inside `_sgvp/fixtures/bad/...` (intentionally bad code used for
// scanner self-tests) don't pollute the bug tracker.
func isSGVPSkipPath(file string) bool {
	low := strings.ToLower(file)
	if strings.HasPrefix(low, "_sgvp/") || strings.Contains(low, "/_sgvp/") {
		return true
	}
	if strings.HasPrefix(low, "scanner/") || strings.Contains(low, "/scanner/") {
		return true
	}
	if strings.HasPrefix(low, "state/") || strings.Contains(low, "/state/") {
		return true
	}
	return false
}

// ─── Helpers ───────────────────────────────────────────────────────────

// inferSeverity applies file-type and file-depth heuristics to determine
// bug severity. P3-1 follow-up: smarter than just marker-based.
//
// Rules:
//   - Base severity from marker type (TODO=low, XXX/HACK=medium, FIXME=high)
//   - Bump +1 for critical paths (internal/core/, internal/bft/, cmd/)
//   - Lower -1 for test files (_test.go)
//   - Bump +1 for deeply nested files (>3 path segments)
func inferSeverity(marker, relPath string) bugregistry.Severity {
	// Base severity from marker
	level := 1 // 0=info, 1=low, 2=medium, 3=high, 4=critical
	switch marker {
	case "TODO":
		level = 1
	case "XXX", "HACK":
		level = 2
	case "FIXME":
		level = 3
	}

	// File-type heuristics
	if strings.HasSuffix(relPath, "_test.go") {
		level-- // test files are lower priority
	}

	// Critical path bump — internal/core/ and internal/bft/ are the "batang otak"
	criticalPaths := []string{
		"internal/core/", "internal/bft/", "internal/ownerauth/",
		"internal/tools/", "cmd/flowork/",
	}
	for _, cp := range criticalPaths {
		if strings.HasPrefix(relPath, cp) {
			level++
			break
		}
	}

	// Depth heuristic — deeply nested files are harder to discover
	segments := strings.Count(relPath, "/")
	if segments > 3 {
		level++
	}

	// Clamp to valid range
	if level < 0 {
		level = 0
	}
	if level > 4 {
		level = 4
	}

	severities := []bugregistry.Severity{
		bugregistry.SeverityInfo,
		bugregistry.SeverityLow,
		bugregistry.SeverityMedium,
		bugregistry.SeverityHigh,
		bugregistry.SeverityCritical,
	}
	return severities[level]
}

func heartbeat(agent string, state status.State, action, detail string) {
	_ = status.Write(status.Snapshot{
		Agent:     agent,
		State:     state,
		Action:    action,
		Detail:    detail,
		UpdatedAt: time.Now().UTC(),
		PID:       os.Getpid(),
	})
}
