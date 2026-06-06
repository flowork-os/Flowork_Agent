// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 25 phase 1 scanner endpoints. Anti-escape: target_path
//   harus di dalam shared workspace agent. Phase 2 (background goroutine
//   long scan, GitHub repo scan, ZIP scan inline) → tambah file baru.
//
// scanner.go — Section 25 phase 1: scan + runs + findings + auditors list.

package agentmgr

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/scanner"
)

// ScannerScanHandler — POST /api/agents/scanner/scan?id=<agent>
// Body: {target_path, scan_type}. target_path resolved relative ke shared
// workspace agent (anti-escape).
func ScannerScanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(agentID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid or missing agent id"})
		return
	}
	var body struct {
		TargetPath string `json:"target_path"`
		ScanType   string `json:"scan_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	if body.TargetPath == "" {
		httpx.WriteJSON(w, map[string]any{"error": "target_path required"})
		return
	}
	if body.ScanType == "" {
		body.ScanType = "manual"
	}

	// Resolve target ke dalam shared workspace.
	sharedRoot := filepath.Join(agentFolder(agentID), "workspace")
	target := filepath.Join(sharedRoot, body.TargetPath)
	if rel, rerr := filepath.Rel(sharedRoot, target); rerr != nil || strings.HasPrefix(rel, "..") {
		httpx.WriteJSON(w, map[string]any{"error": "target_path escapes workspace"})
		return
	}

	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()

	runID, err := store.InsertScannerRun(body.ScanType, body.TargetPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}

	res, err := scanner.Run(scanner.RunOptions{Target: target})
	if err != nil {
		_ = store.FinishScannerRun(runID, 0, 0, "fail")
		httpx.WriteJSON(w, map[string]any{"error": err.Error(), "run_id": runID})
		return
	}

	// Convert findings → DB rows.
	dbFindings := make([]agentdb.ScannerFinding, 0, len(res.Findings))
	criticalCount := 0
	for _, f := range res.Findings {
		dbFindings = append(dbFindings, agentdb.ScannerFinding{
			RunID:       runID,
			Auditor:     f.Auditor,
			Severity:    f.Severity,
			FilePath:    relPathTo(sharedRoot, f.FilePath),
			LineNumber:  f.LineNumber,
			Message:     f.Message,
			Snippet:     f.Snippet,
			Remediation: f.Remediation,
		})
		if f.Severity == scanner.SevCritical {
			criticalCount++
		}
	}
	if err := store.InsertScannerFindings(runID, dbFindings); err != nil {
		_ = store.FinishScannerRun(runID, len(dbFindings), criticalCount, "fail")
		httpx.WriteJSON(w, map[string]any{"error": err.Error(), "run_id": runID})
		return
	}
	status := "pass"
	if criticalCount > 0 {
		status = "fail"
	}
	_ = store.FinishScannerRun(runID, len(dbFindings), criticalCount, status)

	_ = context.TODO()
	httpx.WriteJSON(w, map[string]any{
		"run_id":         runID,
		"files_scanned":  res.FilesScanned,
		"bytes_scanned":  res.BytesScanned,
		"total_findings": len(dbFindings),
		"critical_count": criticalCount,
		"status":         status,
	})
}

func relPathTo(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil {
		return rel
	}
	return abs
}

// ScannerRunsHandler — GET /api/agents/scanner/runs?id=&limit=
func ScannerRunsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListScannerRuns(limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

// ScannerFindingsHandler — GET /api/agents/scanner/findings?id=&run_id=
func ScannerFindingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	runID, _ := strconv.ParseInt(r.URL.Query().Get("run_id"), 10, 64)
	if runID == 0 {
		httpx.WriteJSON(w, map[string]any{"error": "run_id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListScannerFindings(runID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

// ScannerAuditorsHandler — GET /api/agents/scanner/auditors
func ScannerAuditorsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	// auditor statis + tool nyata (trivy imun + nmap/nuclei/dst) → count ga stuck.
	names := append(append([]string{}, scanner.Names()...), scanner.ToolNames()...)
	httpx.WriteJSON(w, map[string]any{
		"items": names,
		"count": len(names),
	})
}
