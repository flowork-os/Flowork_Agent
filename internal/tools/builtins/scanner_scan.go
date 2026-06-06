// scanner_scan.go — AI-INITIATED defensive code scan (Section 25 + immune).
//
// Lets an agent run the SAME static-auditor + trivy pipeline as the background
// watcher, on demand, over its OWN workspace (or a subpath). Findings land in
// the agent state.db (scanner_runs/scanner_findings) → show up in Threat Radar
// and are queryable via scanner_findings_query. The agent runs it, reads the
// summary, then acts — no manual scanning.
//
// DEFENSIVE ONLY: scope is the agent's shared workspace (anti-escape). Offensive
// target scans (nmap/nuclei against external hosts) stay owner-gated in
// scan_exec.go — an agent tool never reaches that gate.

package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/scanner"
	"flowork-gui/internal/tools"
)

func init() {
	tools.Register(&codeScanTool{})
}

type codeScanTool struct{}

func (codeScanTool) Name() string       { return "code_scan" }
func (codeScanTool) Capability() string { return "state:write" }
func (codeScanTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Run a DEFENSIVE security scan (static auditors + trivy: CVE/secret/IaC misconfig) over your workspace, or a subpath. Stores findings (queryable via scanner_findings_query) and returns a ranked summary so you can act on them. Defensive only — scope is your own workspace.",
		Params: []tools.Param{
			{Name: "path", Type: tools.ParamString, Description: "Subpath inside your workspace to scan (default: whole workspace)", Required: false},
		},
		Returns: "{run_id, files_scanned, total_findings, critical, by_severity, status, top[]}",
	}
}

func (codeScanTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok || store == nil {
		return tools.Result{}, fmt.Errorf("agent store not available")
	}
	shared := tools.FromSharedDir(ctx)
	if shared == "" {
		return tools.Result{}, fmt.Errorf("workspace not available")
	}

	rel, _ := args["path"].(string)
	rel = strings.TrimSpace(rel)
	target := shared
	if rel != "" {
		target = filepath.Join(shared, rel)
		if !strings.HasPrefix(target, shared+string(os.PathSeparator)) && target != shared {
			return tools.Result{}, fmt.Errorf("path escapes workspace")
		}
	}
	label := rel
	if label == "" {
		label = "workspace"
	}

	runID, err := store.InsertScannerRun("manual:code_scan", label)
	if err != nil {
		return tools.Result{}, fmt.Errorf("insert run: %w", err)
	}
	res, err := scanner.Run(scanner.RunOptions{Target: target})
	if err != nil {
		_ = store.FinishScannerRun(runID, 0, 0, "fail")
		return tools.Result{}, fmt.Errorf("scan: %w", err)
	}
	// Real-tool layer (trivy) — same as the background watcher / baseline.
	res.Findings = append(res.Findings, scanner.ToolScan(target)...)

	bySev := map[string]int{}
	crit := 0
	dbF := make([]agentdb.ScannerFinding, 0, len(res.Findings))
	for _, f := range res.Findings {
		bySev[f.Severity]++
		if f.Severity == scanner.SevCritical {
			crit++
		}
		fp := f.FilePath
		if r, e := filepath.Rel(shared, f.FilePath); e == nil && !strings.HasPrefix(r, "..") {
			fp = r
		}
		dbF = append(dbF, agentdb.ScannerFinding{
			RunID: runID, Auditor: f.Auditor, Severity: f.Severity,
			FilePath: fp, LineNumber: f.LineNumber, Message: f.Message,
			Snippet: f.Snippet, Remediation: f.Remediation,
		})
	}
	if err := store.InsertScannerFindings(runID, dbF); err != nil {
		_ = store.FinishScannerRun(runID, len(dbF), crit, "fail")
		return tools.Result{}, fmt.Errorf("insert findings: %w", err)
	}
	status := "pass"
	if crit > 0 {
		status = "fail"
	}
	_ = store.FinishScannerRun(runID, len(dbF), crit, status)

	return tools.Result{
		Output: map[string]any{
			"run_id":         runID,
			"files_scanned":  res.FilesScanned,
			"total_findings": len(dbF),
			"critical":       crit,
			"by_severity":    bySev,
			"status":         status,
			"top":            topFindings(dbF, 20),
		},
	}, nil
}

// topFindings — N findings paling parah (severity order), bentuk ringkas biar
// AI bisa langsung beraksi tanpa query ulang per-run.
func topFindings(fs []agentdb.ScannerFinding, n int) []map[string]any {
	order := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
	sorted := append([]agentdb.ScannerFinding(nil), fs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return order[sorted[i].Severity] < order[sorted[j].Severity]
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	out := make([]map[string]any, 0, len(sorted))
	for _, f := range sorted {
		out = append(out, map[string]any{
			"severity": f.Severity, "auditor": f.Auditor,
			"file": f.FilePath, "line": f.LineNumber, "message": f.Message,
		})
	}
	return out
}
