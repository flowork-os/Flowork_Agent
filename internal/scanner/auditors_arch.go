// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Plug-in auditor arsitektur — dari laporan bug eksternal (bug.md),
//   diverifikasi real, belum ada auditornya. (1) staged_path_gate: existence
//   gate cuma cek staged folder, source-agent ke-tolak. (2) db_open_per_call:
//   open state.db berulang di fungsi per-pesan (perf/lock). Function-scope
//   tracking → low FP. Daftar via init(), ga sentuh auditors.go locked.

package scanner

import (
	"regexp"
	"strings"
)

// Anti-pattern existence-gate staged-only. Dua bentuk:
//   (a) inline:    os.Stat(agentFolder(id))
//   (b) dua baris: dir := agentFolder(id) ; ... ; os.Stat(dir)
// agentFolder() nunjuk ke staged (~/.flowork/agents/<id>.fwagent) doang →
// source-agent di repo (agents/<id>/) ke-tolak "not found".
var stagedGateInlineRe = regexp.MustCompile(`os\.Stat\(\s*agentFolder\(`)
var folderVarRe = regexp.MustCompile(`([A-Za-z0-9_]+)\s*:?=\s*agentFolder\(`)
var statVarRe = regexp.MustCompile(`os\.Stat\(\s*([A-Za-z0-9_]+)\s*\)`)

// funcDeclRe — nangkep nama fungsi buat scope tracking (line-based).
var funcDeclRe = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z0-9_]+)\s*\(`)

// perCallFuncRe — nama fungsi yang KEMUNGKINAN dipanggil sering (per pesan/
// per event/per tick) — di situ open/close DB berulang = perf+lock risk.
var perCallFuncRe = regexp.MustCompile(`(?i)^(log|on|tick|fire|notify|poll|handle|dispatch|emit)`)

// dbOpenRe — buka koneksi DB.
var dbOpenRe = regexp.MustCompile(`\b(agentdb\.Open|sql\.Open|floworkdb\.Open)\(`)

// sourceCheckRe — tanda fungsi udah source-aware (cek source repo). Kalau ada
// ini di fungsi yang sama, staged gate-nya BUKAN bug (itu fallback yang benar).
var sourceCheckRe = regexp.MustCompile(`agentSourceDir\(|resolveAgentDir\(|ProjectRoot\(|os\.Getwd\(`)

// AuditStagedPathGate — flag existence gate staged-only (inline + 2-baris),
// TAPI cuma kalau fungsi-nya ngga punya source-check (function-scoped). Skip
// baris definisi regex (anti self-match auditor).
func AuditStagedPathGate(filePath, content string) []Finding {
	var out []Finding
	type hit struct {
		ln   int
		line string
	}
	folderVars := map[string]bool{}
	sawSource := false
	var pending []hit
	flush := func() {
		if !sawSource {
			for _, h := range pending {
				out = append(out, Finding{
					Auditor:     "staged_path_gate_auditor",
					Severity:    SevMedium,
					FilePath:    filePath,
					LineNumber:  h.ln,
					Message:     "existence gate cuma cek staged folder (agentFolder) tanpa cek source repo — source-agent di agents/<id>/ bisa ke-tolak 'not found'",
					Snippet:     snippetOf(h.line),
					Remediation: "cek source dulu (agentSourceDir/resolveAgentDir/agentdb.Resolve) sebelum fallback ke staged loader.AgentsDir().",
				})
			}
		}
		pending = nil
		folderVars = map[string]bool{}
		sawSource = false
	}
	for i, line := range strings.Split(content, "\n") {
		if funcDeclRe.MatchString(line) {
			flush() // ganti fungsi → evaluasi fungsi sebelumnya
		}
		t := strings.TrimSpace(line)
		// Skip komentar (contoh pola di doc) + baris definisi regex (anti self-match).
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") ||
			strings.Contains(line, "MustCompile") || strings.Contains(line, "regexp.") {
			continue
		}
		if sourceCheckRe.MatchString(line) {
			sawSource = true
		}
		if m := folderVarRe.FindStringSubmatch(line); m != nil {
			folderVars[m[1]] = true
		}
		if stagedGateInlineRe.MatchString(line) {
			pending = append(pending, hit{i + 1, line})
			continue
		}
		if m := statVarRe.FindStringSubmatch(line); m != nil && folderVars[m[1]] {
			pending = append(pending, hit{i + 1, line})
		}
	}
	flush()
	return out
}

// AuditDBOpenPerCall — flag DB Open di dalam fungsi yang namanya per-call.
func AuditDBOpenPerCall(filePath, content string) []Finding {
	var out []Finding
	curFunc := ""
	for i, line := range strings.Split(content, "\n") {
		if m := funcDeclRe.FindStringSubmatch(line); m != nil {
			curFunc = m[1]
		}
		if curFunc != "" && perCallFuncRe.MatchString(curFunc) && dbOpenRe.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "db_open_per_call_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "DB Open di fungsi per-call `" + curFunc + "` — open/close berulang tiap pesan/event = latency + lock contention (WAL)",
				Snippet:     snippetOf(line),
				Remediation: "cache *Store per pluginID (sync.Map) atau reuse handle, jangan Open+Close tiap panggilan di hot-path.",
			})
		}
	}
	return out
}

func init() {
	Auditors["staged_path_gate_auditor"] = AuditStagedPathGate
	Auditors["db_open_per_call_auditor"] = AuditDBOpenPerCall
}
