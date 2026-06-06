// scan_exec.go — GATED-EXEC enforcement buat scan tool (roadmap immune P2).
//
// SETIAP scan tool eksternal WAJIB lewat sini. Pertahanan berlapis:
//  1. BLOCKLIST hardcoded — binary destruktif/shell/interpreter GA PERNAH jalan,
//     walau owner salah allowlist (jaga-jaga fat-finger).
//  2. ALLOWLIST exec — binary HARUS di-allowlist owner (default DENY).
//  3. ALLOWLIST target — kalau ada target, HARUS in-scope (default DENY).
//  4. NO SHELL — exec.Command arg-array (bukan sh -c) → nol injection.
//  5. Timeout + output cap. Audit log tiap run.
//
//	POST /api/scanner/run {binary, args[], target}  → owner-only loopback.
//
// Owner pegang gerbang (allowlist); AGENT ga punya akses endpoint ini.

package scanapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/routerclient"
)

// scanExecBlocklist — binary yang DILARANG MUTLAK (defense-in-depth). Cek basename
// → walau owner allowlist "rm" / kasih "/bin/rm", tetep DITOLAK. Shell + interpreter
// di-block biar gerbang ga bisa di-bypass jadi arbitrary-code-exec.
var scanExecBlocklist = map[string]bool{
	// destruktif
	"rm": true, "rmdir": true, "dd": true, "mkfs": true, "mke2fs": true, "shred": true,
	"format": true, "fdisk": true, "parted": true, "wipefs": true, "blkdiscard": true,
	// kontrol sistem
	"shutdown": true, "reboot": true, "halt": true, "poweroff": true, "init": true, "systemctl": true,
	// privilege / user / mount
	"sudo": true, "su": true, "doas": true, "chmod": true, "chown": true, "chgrp": true,
	"useradd": true, "usermod": true, "userdel": true, "passwd": true, "mount": true, "umount": true,
	"crontab": true, "at": true,
	// shell + interpreter (anti shell-escape / arbitrary code)
	"sh": true, "bash": true, "zsh": true, "fish": true, "csh": true, "ksh": true, "dash": true,
	"python": true, "python3": true, "perl": true, "ruby": true, "node": true, "nodejs": true,
	"php": true, "lua": true, "eval": true, "env": true, "xargs": true, "find": true,
}

// gatedScanRun — eksekusi 1 scan tool LEWAT GERBANG. Balik (stdout, stderr, denied).
// denied != "" → DITOLAK (ga dijalanin). err != nil → gagal jalan/timeout.
// hostRe matches a bare FQDN (a.b.c.tld) so a plain domain arg is treated as a
// scan target that must be allowlisted.
var hostRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// hostsInArg extracts the scan target(s) carried by one arg: a URL host, a bare
// domain, an IP, or a CIDR. Flags (leading '-') and non-host values yield nothing,
// so the allowlist check below only fires on real destinations.
func hostsInArg(a string) []string {
	a = strings.TrimSpace(a)
	if a == "" || strings.HasPrefix(a, "-") {
		return nil
	}
	if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
		if u, err := url.Parse(a); err == nil && u.Hostname() != "" {
			return []string{u.Hostname()}
		}
		return nil
	}
	if _, _, err := net.ParseCIDR(a); err == nil {
		return []string{a}
	}
	// strip a path then an optional :port to get the bare host.
	h := a
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	if net.ParseIP(h) != nil || hostRe.MatchString(h) {
		return []string{h}
	}
	return nil
}

func gatedScanRun(store *floworkdb.Store, binary string, args []string, target string) (stdout, stderr, denied string, exit int, err error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return "", "", "binary kosong", 0, nil
	}
	base := strings.ToLower(filepath.Base(binary))

	// 1) BLOCKLIST hardcoded (paling atas — ga bisa di-override allowlist).
	if scanExecBlocklist[base] {
		return "", "", "DITOLAK: binary '" + base + "' di-blocklist permanen (destruktif/shell) — ga bisa di-allowlist", 0, nil
	}
	// 2) ALLOWLIST exec (default DENY).
	ok, aerr := store.IsAllowed("exec", binary)
	if aerr != nil {
		return "", "", "", 0, aerr
	}
	if !ok {
		return "", "", "DITOLAK: binary '" + binary + "' ga ada di allowlist exec (owner belum izinin)", 0, nil
	}
	// 3) ALLOWLIST target (kalau ada).
	target = strings.TrimSpace(target)
	if target != "" {
		tok, terr := store.IsAllowed("target", target)
		if terr != nil {
			return "", "", "", 0, terr
		}
		if !tok {
			return "", "", "DITOLAK: target '" + target + "' di luar scope allowlist (owner belum izinin)", 0, nil
		}
	}
	// arg sanity: no null byte, cap jumlah + panjang.
	if len(args) > 64 {
		return "", "", "DITOLAK: args terlalu banyak (>64)", 0, nil
	}
	for _, a := range args {
		if strings.ContainsRune(a, 0) {
			return "", "", "DITOLAK: arg ada null byte", 0, nil
		}
	}
	// 3b) ALLOWLIST target di ARGS — destinasi scan sebenarnya ada di sini
	// (-u <url>, -target <host>, CIDR), BUKAN cuma di field `target`. Tiap
	// host/IP/CIDR di args WAJIB in-scope, biar field `target` ga bisa dipake
	// nipu allowlist sambil args nembak host lain (scope/SSRF bypass).
	for _, a := range args {
		for _, host := range hostsInArg(a) {
			hok, herr := store.IsAllowed("target", host)
			if herr != nil {
				return "", "", "", 0, herr
			}
			if !hok {
				return "", "", "DITOLAK: target '" + host + "' (dari args) di luar scope allowlist", 0, nil
			}
		}
	}

	// 4) EKSEKUSI — NO SHELL (arg array), timeout, output cap.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...) // #nosec — binary allowlisted + blocklist + no shell
	var outBuf, errBuf bytes.Buffer
	const cap = 1 << 20 // 1MB cap per stream
	cmd.Stdout = &limitedWriter{w: &outBuf, max: cap}
	cmd.Stderr = &limitedWriter{w: &errBuf, max: cap}
	// audit log.
	fmt.Fprintf(os.Stderr, "[scan-exec] RUN binary=%q args=%v target=%q\n", binary, args, target)

	runErr := cmd.Run()
	exit = 0
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return outBuf.String(), errBuf.String(), "", exit, fmt.Errorf("timeout 120s")
	}
	return outBuf.String(), errBuf.String(), "", exit, runErr
}

// limitedWriter — tulis sampai max byte, sisanya di-drop (anti banjir output).
type limitedWriter struct {
	w   io.Writer
	max int
	n   int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n >= l.max {
		return len(p), nil // drop, tetep "sukses" biar cmd ga error
	}
	rem := l.max - l.n
	if len(p) > rem {
		p = p[:rem]
	}
	n, err := l.w.Write(p)
	l.n += n
	return n, err
}

func ScannerRunHandler(store *floworkdb.Store, openAgent func(string) (*agentdb.Store, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Binary   string   `json:"binary"`
			Args     []string `json:"args"`
			Target   string   `json:"target"`
			Category string   `json:"category"` // immune (defensif) | pentest (ofensif)
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		// ENFORCE uninstall: kalau nuclei, exclude pack yang dicopot (disabled-set) →
		// pack ke-uninstall BENERAN ga ke-scan, bukan cuma ilang dari katalog.
		body.Args = applyNucleiExclusions(store, body.Binary, body.Args)
		stdout, stderr, denied, exit, err := gatedScanRun(store, body.Binary, body.Args, body.Target)
		// audit trail: catat SETIAP percobaan (ran/denied/error) — bahan laporan + jejak.
		rec := floworkdb.ScanRun{Binary: body.Binary, Args: strings.Join(body.Args, " "), Target: body.Target, ExitCode: exit, Stdout: stdout, Stderr: stderr, Denied: denied}
		switch {
		case denied != "":
			rec.Status = "denied"
		case err != nil:
			rec.Status = "error"
			rec.Stderr = strings.TrimSpace(rec.Stderr + "\n" + err.Error())
		default:
			rec.Status = "ran"
		}
		runID, _ := store.AddScanRun(rec)

		if denied != "" {
			tfWriteJSON(w, http.StatusForbidden, map[string]any{"ok": false, "denied": denied, "run_id": runID})
			return
		}
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error(), "stdout": stdout, "stderr": stderr, "run_id": runID})
			return
		}
		// PARSE deterministik (immune P2.2b): output → finding terstruktur → simpan.
		// Cuma tool ke-parse (nmap/nuclei/trivy) yg balikin finding; lainnya nil.
		findings := parseFindings(body.Binary, body.Target, body.Category, stdout)
		nf, _ := store.AddScanFindings(runID, findings)
		// NYATU: active-scan tampil di Threat Radar (scan log + findings) yang SAMA
		// kayak codescan/imun — bukan blok kepisah.
		mirrorActiveScanToRadar(openAgent, body.Binary, body.Target, findings)
		tfWriteJSON(w, 0, map[string]any{
			"ok": true, "exit": exit, "stdout": stdout, "stderr": stderr,
			"run_id": runID, "findings": findings, "findings_count": nf,
		})
	}
}

// mirrorActiveScanToRadar — tulis hasil active-scan ke state.db mr-flow (Scanner
// Run/Finding) → tampil di Threat Radar (scan log "active:<tool>" + findings),
// SATU tampilan sama codescan/imun. Best-effort (ga ganggu response kalau gagal).
func mirrorActiveScanToRadar(openAgent func(string) (*agentdb.Store, error), binary, target string, findings []floworkdb.ScanFinding) {
	if openAgent == nil {
		return
	}
	st, err := openAgent("mr-flow")
	if err != nil {
		return
	}
	defer st.Close()
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(binary), ".exe"))
	tgt := strings.TrimSpace(target)
	if tgt == "" {
		tgt = base
	}
	runID, err := st.InsertScannerRun("active:"+base, tgt)
	if err != nil {
		return
	}
	sf := make([]agentdb.ScannerFinding, 0, len(findings))
	crit := 0
	for _, f := range findings {
		msg := f.Title
		var ex []string
		if f.CVE != "" {
			ex = append(ex, f.CVE)
		}
		if f.CWE != "" {
			ex = append(ex, f.CWE)
		}
		if f.CVSS > 0 {
			ex = append(ex, fmt.Sprintf("CVSS %.1f", f.CVSS))
		}
		if len(ex) > 0 {
			msg += " [" + strings.Join(ex, " ") + "]"
		}
		sf = append(sf, agentdb.ScannerFinding{
			RunID: runID, Auditor: f.Tool, Severity: f.Severity,
			FilePath: f.Target, Message: msg, Remediation: f.Component,
		})
		if f.Severity == "critical" {
			crit++
		}
	}
	_ = st.InsertScannerFindings(runID, sf)
	status := "pass"
	if crit > 0 {
		status = "fail"
	}
	_ = st.FinishScannerRun(runID, len(sf), crit, status)
}

// ScannerFindingsHandler — GET daftar finding terstruktur (laporan). Owner-only loopback.
// ?run_id=N → finding 1 run; tanpa run_id → N terakhir (urut severity).
func ScannerFindingsHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			items []floworkdb.ScanFinding
			err   error
		)
		if rid := strings.TrimSpace(r.URL.Query().Get("run_id")); rid != "" {
			var n int64
			fmt.Sscan(rid, &n)
			items, err = store.ListScanFindingsByRun(n)
		} else {
			items, err = store.ListScanFindings(100)
		}
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		counts, _ := store.CountFindingsBySeverity()
		tfWriteJSON(w, 0, map[string]any{"findings": items, "count": len(items), "by_severity": counts})
	}
}

// ScannerFindingVerifyHandler — POST /api/scanner/findings/verify {id, verified}.
// Owner KONFIRMASI finding (prinsip #6: vuln ga "real" sebelum diverifikasi ulang).
// Buat tool defensif deterministik, verifikasi = owner-driven (manusia konfirmasi,
// bukan auto-rerun) → set slot reproducible_ok. Owner-only loopback.
func ScannerFindingVerifyHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			ID       int64 `json:"id"`
			Verified bool  `json:"verified"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<12)).Decode(&body); err != nil || body.ID == 0 {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
			return
		}
		if err := store.MarkFindingVerified(body.ID, body.Verified); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "id": body.ID, "verified": body.Verified})
	}
}

// triageQuery — derive query FTS PENDEK dari finding (immune P1.3). FTS router
// join token AND → query pendek (1 token kuat) maksimalin hit. Urut sinyal:
// CVE > component-base > token alpha terpanjang di title. Deterministik.
func triageQuery(f floworkdb.ScanFinding) string {
	if f.CVE != "" {
		cve := f.CVE
		if i := strings.IndexByte(cve, ','); i > 0 {
			cve = cve[:i]
		}
		return strings.TrimSpace(cve)
	}
	if f.Component != "" {
		base := f.Component
		if i := strings.IndexAny(base, "@ "); i > 0 {
			base = base[:i]
		}
		if base = strings.TrimSpace(base); len(base) >= 3 {
			return base
		}
	}
	best := ""
	for _, tok := range strings.Fields(strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return ' '
	}, f.Title)) {
		if len(tok) > len(best) {
			best = tok
		}
	}
	return best
}

// ScannerPushHandler — POST /api/scanner/findings/push?id=N. SYNC finding ke
// TRACKER RESMI di brain Router (laporan jasa keamanan):
//
//	category immune  → POST router /api/brain/immune/add  (immune_system)
//	category pentest → POST router /api/brain/pentest/add (pentest_karma)
//
// Router yang nulis brain (anti tembak DB 32GB langsung). Owner-only loopback.
func ScannerPushHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var fid int64
		fmt.Sscan(strings.TrimSpace(r.URL.Query().Get("id")), &fid)
		if fid == 0 {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id wajib"})
			return
		}
		f, err := store.GetScanFinding(fid)
		if err != nil {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		base := routerclient.DefaultRouterURL
		var endpoint string
		var payload map[string]any
		if f.Category == "pentest" {
			endpoint = base + "/api/brain/pentest/add"
			meta, _ := json.Marshal(map[string]any{"tool": f.Tool, "run_id": f.RunID, "component": f.Component})
			payload = map[string]any{
				"target": f.Target, "finding_title": f.Title, "cwe_id": f.CWE,
				"cvss_score": f.CVSS, "severity": f.Severity,
				"evidence_url": f.Evidence, "reproduction_steps": "tool=" + f.Tool + " component=" + f.Component + " meta=" + string(meta),
			}
		} else {
			endpoint = base + "/api/brain/immune/add"
			meta, _ := json.Marshal(map[string]any{"tool": f.Tool, "cve": f.CVE, "cwe": f.CWE, "cvss": f.CVSS, "component": f.Component, "target": f.Target, "run_id": f.RunID, "finding_id": f.ID})
			payload = map[string]any{
				"type": "scan", "name": f.Tool + ": " + f.Title + " [" + f.Target + "]",
				"severity": f.Severity, "status": "open",
				"description": f.Component + " · " + f.Evidence, "meta_data": string(meta),
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		out, perr := routerPostJSON(ctx, endpoint, payload)
		if perr != nil {
			tfWriteJSON(w, http.StatusBadGateway, map[string]any{"error": "router push gagal: " + perr.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "category": normScanCategory(f.Category), "tracker_id": out["id"]})
	}
}

// ScannerTrackersHandler — GET /api/scanner/trackers. Dashboard laporan: proxy
// list immune_system + pentest_karma dari Router (read-only). Owner-only loopback.
func ScannerTrackersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := routerclient.DefaultRouterURL
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		get := func(path string) []any {
			m, err := routerGetJSON(ctx, base+path)
			if err != nil {
				return []any{}
			}
			if items, ok := m["items"].([]any); ok {
				return items
			}
			return []any{}
		}
		immune := get("/api/brain/immune/list?limit=200")
		pentest := get("/api/brain/pentest/list?limit=200")
		tfWriteJSON(w, 0, map[string]any{
			"immune": immune, "pentest": pentest,
			"immune_count": len(immune), "pentest_count": len(pentest),
		})
	}
}

// routerGetJSON — GET JSON dari Router → map. Helper dashboard tracker.
func routerGetJSON(ctx context.Context, url string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil, fmt.Errorf("router resp %d", resp.StatusCode)
	}
	return out, nil
}

// routerPostJSON — POST JSON ke Router, balik map response. Helper push tracker.
func routerPostJSON(ctx context.Context, url string, payload map[string]any) (map[string]any, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil, fmt.Errorf("router resp %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if ev, ok := out["error"].(string); ok && ev != "" {
		return nil, fmt.Errorf("%s", ev)
	}
	return out, nil
}

// ScannerTriageHandler — GET /api/scanner/findings/triage?id=N (atau ?q=term).
// RAG-assist (immune P1.3): query 5jt drawer Router (FTS BM25, DETERMINISTIK,
// NO LLM) → konteks/teknik/eksploitasi buat finding. Owner-only loopback.
func ScannerTriageHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		var fid int64
		if q == "" {
			idStr := strings.TrimSpace(r.URL.Query().Get("id"))
			if idStr == "" {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id atau q wajib"})
				return
			}
			fmt.Sscan(idStr, &fid)
			f, err := store.GetScanFinding(fid)
			if err != nil {
				tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
				return
			}
			q = triageQuery(f)
		}
		if q == "" {
			tfWriteJSON(w, 0, map[string]any{"finding_id": fid, "query": "", "hits": []any{}, "count": 0, "note": "ga ada term buat query"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		client := routerclient.New(routerclient.DefaultRouterURL)
		resp, err := client.SearchBrain(ctx, q, 5)
		if err != nil {
			tfWriteJSON(w, http.StatusBadGateway, map[string]any{"error": "brain ga kejangkau: " + err.Error(), "query": q})
			return
		}
		hits := make([]map[string]any, 0, len(resp.Hits))
		for _, h := range resp.Hits {
			c := h.Content
			if len(c) > 400 {
				c = c[:400] + "…"
			}
			hits = append(hits, map[string]any{"wing": h.Wing, "room": h.Room, "excerpt": c, "score": h.Score, "drawer_id": h.DrawerID})
		}
		tfWriteJSON(w, 0, map[string]any{"finding_id": fid, "query": q, "hits": hits, "count": len(hits)})
	}
}

// ScannerRunsHandler — GET daftar run terakhir (audit/history).
func ScannerRunsHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runs, err := store.ListScanRuns(50)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"runs": runs, "count": len(runs)})
	}
}
