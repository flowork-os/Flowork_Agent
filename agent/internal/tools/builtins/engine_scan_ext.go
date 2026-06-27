// engine_scan_ext.go — FROZEN (chattr +i + hash KERNEL_FREEZE.md). 📄 Dok: lock/threat-radar.md.
// Tool `engine_scan`: scan KODE INTI Flowork (engine FLowork_os: agent/router/os) buat bug/security.
// BEDA FUNGSI dari `code_scan` (frozen, scope workspace agent) DAN dari `codemap_*` (peta struktur).
// Ini AUDIT bug di engine sendiri.
//
// ⚠️ GATE owner-class: cap = "exec:git" (mr-flow PUNYA; worker browse/fb-* TIDAK). Sengaja —
// kalau pakai cap lebar (fs:read), worker lain pas cari job-nya bisa NYASAR scan engine-root
// (langgar batas workspace). Engine-scan = privileged, cuma owner-class.
//
// Cabut-akar (Rule #7): code_scan FROZEN + scope-workspace by-design → JANGAN edit. Tambah tool
// BARU lewat seam. Reuse scanner.Run + ToolScan (auditor statik yg sama). Host-side (no wasm).
// No-hardcode (Rule #6): engine dir di-resolve via env / executable / cwd, bukan path literal.
package builtins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/scanner"
	"flowork-gui/internal/tools"
)

func init() { tools.Register(&engineScanTool{}) }

type engineScanTool struct{}

func (engineScanTool) Name() string       { return "engine_scan" }
func (engineScanTool) Capability() string { return "exec:git" } // gate owner-class — worker ga punya, biar ga nyasar
func (engineScanTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Scan KODE INTI Flowork (engine: agent/router/os di FLowork_os) buat bug + security pakai " +
			"auditor statik. BEDA dari `code_scan` — itu scan workspace agent yang sering KOSONG. Pakai INI buat " +
			"'cek bug di Flowork / scan engine'. Butuh source engine ada (mode dev). " +
			"Return {engine_dir, files_scanned, total_findings, critical, by_severity, status, top[]}.",
		Params: []tools.Param{
			{Name: "path", Type: tools.ParamString, Description: "subpath di engine (mis 'agent' atau 'router/internal/mesh'); kosong = seluruh engine", Required: false},
		},
		Returns: "{engine_dir, files_scanned, total_findings, critical, by_severity, status, top[]}",
	}
}

func (engineScanTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	dir := engineDir()
	if dir == "" {
		return tools.Result{}, fmt.Errorf("source engine FLowork_os ga ketemu — set env FLOWORK_ENGINE_DIR, atau jalanin dari source (portable/img ga bawa source)")
	}
	target := dir
	if rel, _ := args["path"].(string); strings.TrimSpace(rel) != "" {
		target = filepath.Clean(filepath.Join(dir, filepath.FromSlash(strings.TrimSpace(rel))))
		if c, e := filepath.Rel(dir, target); e != nil || strings.HasPrefix(c, "..") {
			return tools.Result{}, fmt.Errorf("path keluar engine: %v", rel)
		}
	}

	res, err := scanner.Run(scanner.RunOptions{Target: target})
	if err != nil {
		return tools.Result{}, fmt.Errorf("scan engine: %w", err)
	}
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
		if r, e := filepath.Rel(dir, f.FilePath); e == nil && !strings.HasPrefix(r, "..") {
			fp = r // path relatif engine (rapi + ga bocorin /home/...)
		}
		dbF = append(dbF, agentdb.ScannerFinding{
			Auditor: f.Auditor, Severity: f.Severity, FilePath: fp,
			LineNumber: f.LineNumber, Message: f.Message, Snippet: f.Snippet, Remediation: f.Remediation,
		})
	}
	status := "pass"
	if crit > 0 {
		status = "fail"
	}
	return tools.Result{Output: map[string]any{
		"engine_dir":     dir,
		"files_scanned":  res.FilesScanned,
		"total_findings": len(dbF),
		"critical":       crit,
		"by_severity":    bySev,
		"status":         status,
		"top":            topFindings(dbF, 20), // reuse helper di scanner_scan.go (package sama)
	}}, nil
}

// engineDir — resolve root engine FLowork_os (punya agent/ + router/ + os/). No-hardcode:
// env FLOWORK_ENGINE_DIR → turunin dari executable → cwd & parent-nya. "" kalau ga ketemu.
func engineDir() string {
	if d := strings.TrimSpace(os.Getenv("FLOWORK_ENGINE_DIR")); d != "" && isEngineDir(d) {
		return d
	}
	cands := []string{}
	if exe, err := os.Executable(); err == nil {
		// .../FLowork_os/agent/bin/flowork-gui → naik sampe ketemu engine root
		d := filepath.Dir(exe)
		for i := 0; i < 5; i++ {
			cands = append(cands, d)
			d = filepath.Dir(d)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		cands = append(cands, cwd, filepath.Dir(cwd), filepath.Dir(filepath.Dir(cwd)))
	}
	for _, c := range cands {
		if isEngineDir(c) {
			return c
		}
	}
	return ""
}

func isEngineDir(d string) bool {
	for _, m := range []string{"agent", "router", "os"} {
		st, err := os.Stat(filepath.Join(d, m))
		if err != nil || !st.IsDir() {
			return false
		}
	}
	return true
}
