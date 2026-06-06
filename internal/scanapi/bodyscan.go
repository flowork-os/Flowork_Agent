// bodyscan.go — "Scan Tubuh Flowork": orkestrasi scan KODE semua repo Flowork
// (auditor statis + trivy) → tulis ke state.db mr-flow (ScannerRun + Findings) →
// MUNCUL di Threat Radar (radar + scan log + findings), jalur SAMA kayak codescan.
// Owner-only loopback. READ-ONLY ke kode (cuma baca). NOL token (deterministik).
//
//	POST /api/scanner/bodyscan {roots[]?}  → posture per-repo + run_id (ke radar)

package scanapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/scanner"
)

type bodyFinding struct {
	Severity string `json:"severity"`
	Auditor  string `json:"auditor"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

type bodyRepo struct {
	Root       string         `json:"root"`
	RunID      int64          `json:"run_id"`
	Files      int            `json:"files"`
	Total      int            `json:"total"`
	BySeverity map[string]int `json:"by_severity"`
	Top        []bodyFinding  `json:"top"` // cuma critical+high (yg penting diliat dulu)
}

var sevRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}

// safeBodyScanRoot rejects sensitive/system directories so a body scan can't be
// pointed at the whole filesystem or system paths (which would slurp secrets into
// the findings DB). Owner-only feature, but defense-in-depth for production.
func safeBodyScanRoot(root string) bool {
	abs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	if abs == "/" {
		return false
	}
	for _, deny := range []string{"/etc", "/root", "/usr", "/var", "/sys", "/proc", "/boot", "/bin", "/sbin", "/lib", "/lib64", "/dev", "/opt", "/srv"} {
		if abs == deny || strings.HasPrefix(abs, deny+string(filepath.Separator)) {
			return false
		}
	}
	if home, herr := os.UserHomeDir(); herr == nil && filepath.Clean(home) == abs {
		return false // the home dir root itself is too broad — scan a repo under it
	}
	return true
}

// scanOneRoot — scan 1 repo (auditor + trivy) → posture + SEMUA finding (urut severity).
func scanOneRoot(root string) (bodyRepo, []scanner.Finding) {
	br := bodyRepo{Root: root, BySeverity: map[string]int{}}
	res, err := scanner.Run(scanner.RunOptions{Target: root})
	if err != nil {
		return br, nil
	}
	all := append(res.Findings, scanner.ToolScan(root)...)
	sort.SliceStable(all, func(i, j int) bool { return sevRank[all[i].Severity] < sevRank[all[j].Severity] })
	br.Files = res.FilesScanned
	br.Total = len(all)
	for _, f := range all {
		br.BySeverity[f.Severity]++
		if (f.Severity == "critical" || f.Severity == "high") && len(br.Top) < 60 {
			msg := f.Message
			if len(msg) > 130 {
				msg = msg[:130]
			}
			br.Top = append(br.Top, bodyFinding{
				Severity: f.Severity, Auditor: f.Auditor,
				File: relTo(root, f.FilePath), Line: f.LineNumber, Message: msg,
			})
		}
	}
	return br, all
}

func relTo(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

// writeRadar — tulis hasil 1 repo ke state.db mr-flow → tampil di Threat Radar
// (scan log "body:<repo>" + findings + radar). Best-effort. Return run_id.
func writeRadar(openAgent func(string) (*agentdb.Store, error), root string, all []scanner.Finding, crit int) int64 {
	if openAgent == nil {
		return 0
	}
	st, err := openAgent("mr-flow")
	if err != nil {
		return 0
	}
	defer st.Close()
	runID, err := st.InsertScannerRun("body:"+filepath.Base(root), root)
	if err != nil {
		return 0
	}
	dbF := make([]agentdb.ScannerFinding, 0, len(all))
	for _, f := range all {
		dbF = append(dbF, agentdb.ScannerFinding{
			RunID: runID, Auditor: f.Auditor, Severity: f.Severity,
			FilePath: relTo(root, f.FilePath), LineNumber: f.LineNumber,
			Message: f.Message, Snippet: f.Snippet, Remediation: f.Remediation,
		})
	}
	_ = st.InsertScannerFindings(runID, dbF)
	status := "pass"
	if crit > 0 {
		status = "fail"
	}
	_ = st.FinishScannerRun(runID, len(dbF), crit, status)
	return runID
}

// ScannerBodyScanHandler — POST {roots[]}: scan tiap repo → tulis ke radar → agregasi.
// Default roots = [cwd]. Owner kasih [agent, router, ...] buat tubuh penuh.
func ScannerBodyScanHandler(openAgent func(string) (*agentdb.Store, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Roots []string `json:"roots"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body)
		roots := body.Roots
		if len(roots) == 0 {
			if cwd, e := os.Getwd(); e == nil {
				roots = []string{cwd}
			}
		}
		repos := make([]bodyRepo, 0, len(roots))
		totals := map[string]int{}
		totalFiles := 0
		for _, root := range roots {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if !safeBodyScanRoot(root) {
				repos = append(repos, bodyRepo{Root: root, BySeverity: map[string]int{"_denied": 1}})
				continue
			}
			if st, e := os.Stat(root); e != nil || !st.IsDir() {
				repos = append(repos, bodyRepo{Root: root, BySeverity: map[string]int{"_error": 1}})
				continue
			}
			br, all := scanOneRoot(root)
			br.RunID = writeRadar(openAgent, root, all, br.BySeverity["critical"])
			for k, v := range br.BySeverity {
				totals[k] += v
			}
			totalFiles += br.Files
			repos = append(repos, br)
		}
		tfWriteJSON(w, 0, map[string]any{
			"ok": true, "repos": repos, "totals": totals,
			"total_files": totalFiles, "repo_count": len(repos),
		})
	}
}
