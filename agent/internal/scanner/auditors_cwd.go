// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"regexp"
	"strings"
)

var getwdRe = regexp.MustCompile(`\bos\.Getwd\s*\(`)

func AuditCwdDependency(filePath, content string) []Finding {
	var out []Finding
	for i, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") {
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
