// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"path/filepath"
	"strings"
)

var lockableExt = map[string]bool{".go": true, ".js": true, ".ts": true}

func AuditUnlockedFile(filePath, content string) []Finding {
	ext := strings.ToLower(filepath.Ext(filePath))
	if !lockableExt[ext] || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}

	head := content
	if len(head) > 2000 {
		head = head[:2000]
	}
	if strings.Contains(head, "=== LOCKED FILE ===") {
		return nil
	}
	return []Finding{{
		Auditor:     "unlocked_file_auditor",
		Severity:    SevInfo,
		FilePath:    filePath,
		LineNumber:  1,
		Message:     "file belum di-LOCK — prioritas hunting bug di sini (file ber-header LOCKED = udah diaudit/stable)",
		Snippet:     "",
		Remediation: "kalau udah teruji + stabil, tambah header `=== LOCKED FILE ===` biar ke-skip dari fokus bug-hunt.",
	}}
}

func init() {
	Auditors["unlocked_file_auditor"] = AuditUnlockedFile
}
