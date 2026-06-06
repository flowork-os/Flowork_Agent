// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Plug-in auditor — path resolution bergantung os.Getwd() (rapuh
//   kalau binary dijalankan dari cwd lain → source-agent salah resolve).
//   Dari laporan bug eksternal (bug.md), diverifikasi real. Low FP (os.Getwd
//   jarang). Daftar via init(), ga sentuh auditors.go locked.

package scanner

import (
	"regexp"
	"strings"
)

var getwdRe = regexp.MustCompile(`\bos\.Getwd\s*\(`)

// AuditCwdDependency — flag os.Getwd() yang dipakai resolve path. Advisory:
// path harusnya dari root eksplisit / env override, bukan cwd runtime.
func AuditCwdDependency(filePath, content string) []Finding {
	var out []Finding
	for i, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") { // skip komentar
			continue
		}
		if getwdRe.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "cwd_dependency_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "path resolution via os.Getwd() — rapuh kalau binary dijalankan dari cwd lain (source-agent bisa salah resolve)",
				Snippet:     snippetOf(line),
				Remediation: "pakai root eksplisit / env override (mis. FLOWORK_*_ROOT) sebagai sumber kebenaran lokasi, jangan andelin working directory runtime.",
			})
		}
	}
	return out
}

func init() {
	Auditors["cwd_dependency_auditor"] = AuditCwdDependency
}
