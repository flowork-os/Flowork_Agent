// imports_antigravity.go — SIBLING ext (deletable, NON-frozen): colok detektor
// OAuth import Antigravity + Gemini CLI ke papan RegisterDetector (imports.go).
// Muncul otomatis di dropdown OAuth Imports GUI (DetectAll). Plug-and-play:
// hapus file → CLI-nya ilang dari daftar, core utuh. 📄 Dok: lock/antigravity.md
package creds

import (
	"os"
	"path/filepath"
)

func init() {
	RegisterDetector(detectAntigravity)
	RegisterDetector(detectGeminiCLI)
}

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
