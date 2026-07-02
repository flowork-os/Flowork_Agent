// imports_antigravity.go — SIBLING ext (⚠️ FROZEN 2026-07-02 seizin owner —
// behavior stabil dikunci). 📄 Dok: lock/connect-prune.md
// Colok detektor OAuth import Antigravity ke papan RegisterDetector (imports.go) +
// isi DetectFilter (sembunyiin import untested not-found). Switch env
// FLOWORK_IMPORT_PRUNE=0 → tampilin semua (ubah behavior TANPA buka gembok).
// Re-enable gemini-cli / detektor lain = perubahan struktural → butuh unlock.
package creds

import (
	"os"
	"path/filepath"
)

func init() {
	RegisterDetector(detectAntigravity)
	// gemini-cli DIHAPUS (owner: CLI untested — ga punya buat dites). Balik: uncomment.
	// RegisterDetector(detectGeminiCLI)

	// FILTER (owner 2026-07-02): OAuth Imports cuma nampilin yg FOUND (proven) —
	// sembunyiin CLI untested not-found (codex/cursor/gitlab-duo/dll). Switch env
	// FLOWORK_IMPORT_PRUNE=0 → tampilin semua. Plug-and-play (hapus file → filter ilang).
	DetectFilter = func(in []ImportStatus) []ImportStatus {
		if v := os.Getenv("FLOWORK_IMPORT_PRUNE"); v == "0" || v == "false" {
			return in
		}
		out := make([]ImportStatus, 0, len(in))
		for _, s := range in {
			if s.Found {
				out = append(out, s)
			}
		}
		return out
	}
}

var _ = detectGeminiCLI // simpen fungsi (dipakai lagi kalau owner mau)

// detectAntigravity — app Antigravity ke-detect via folder config-nya. Token
// di-AUTO-CAPTURE lewat MITM (bukan file), jadi 'found' = app-nya kepasang.
func detectAntigravity(home string) ImportStatus {
	s := ImportStatus{Source: "antigravity"}
	candidates := []string{
		filepath.Join(home, ".config", "Antigravity"),
		filepath.Join(home, ".gemini", "antigravity"),
	}
	for _, p := range candidates {
		s.Path = p
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			s.Found = true
			s.MaskedKey = "(token via MITM auto-capture — jalanin app Antigravity sekali)"
			return s
		}
	}
	s.Error = "Antigravity app ga ke-detect — install + login dulu"
	return s
}

// detectGeminiCLI — gemini-cli OAuth (kalau owner pakai). Standar path.
func detectGeminiCLI(home string) ImportStatus {
	p := filepath.Join(home, ".gemini", "oauth_creds.json")
	s := ImportStatus{Source: "gemini-cli", Path: p}
	if _, err := os.Stat(p); err != nil {
		s.Error = "not found — run `gemini` + login"
		return s
	}
	s.Found = true
	s.MaskedKey = "(token present — import via dropdown)"
	return s
}
