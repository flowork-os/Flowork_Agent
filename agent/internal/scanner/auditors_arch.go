// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"regexp"
	"strings"
)

var stagedGateInlineRe = regexp.MustCompile(`os\.Stat\(\s*agentFolder\(`)
var folderVarRe = regexp.MustCompile(`([A-Za-z0-9_]+)\s*:?=\s*agentFolder\(`)
var statVarRe = regexp.MustCompile(`os\.Stat\(\s*([A-Za-z0-9_]+)\s*\)`)

var funcDeclRe = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z0-9_]+)\s*\(`)

var perCallFuncRe = regexp.MustCompile(`(?i)^(log|on|tick|fire|notify|poll|handle|dispatch|emit)`)

var dbOpenRe = regexp.MustCompile(`\b(agentdb\.Open|sql\.Open|floworkdb\.Open)\(`)

var sourceCheckRe = regexp.MustCompile(`agentSourceDir\(|resolveAgentDir\(|ProjectRoot\(|os\.Getwd\(`)

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
			flush()
		}
		t := strings.TrimSpace(line)

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
