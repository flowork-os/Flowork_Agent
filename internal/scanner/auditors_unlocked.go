// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Auditor "file belum di-lock" — file .go/.js/.ts tanpa header
//   `=== LOCKED FILE ===` ditandai (info) biar hunting bug bisa fokus ke file
//   yang belum stable/teruji (locked = udah diaudit). Daftar via init().

package scanner

import (
	"path/filepath"
	"strings"
)

// lockableExt — ekstensi yang pakai konvensi LOCKED header di Flowork.
var lockableExt = map[string]bool{".go": true, ".js": true, ".ts": true}

// AuditUnlockedFile — 1 finding (info) per file lockable yang BELUM ada
// header LOCKED. Skip _test.go (test ngga perlu lock).
func AuditUnlockedFile(filePath, content string) []Finding {
	ext := strings.ToLower(filepath.Ext(filePath))
	if !lockableExt[ext] || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	// Cek header LOCKED di ~25 baris pertama (header selalu di atas).
	head := content
	if len(head) > 2000 {
		head = head[:2000]
	}
	if strings.Contains(head, "=== LOCKED FILE ===") {
		return nil // udah locked → stable
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
