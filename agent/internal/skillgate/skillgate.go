// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package skillgate

import (
	"regexp"
	"strings"
)

var dangerRe = regexp.MustCompile(`(?i)(\brm\s+-rf|\bmkfs\b|:\(\)\s*\{|\bdd\s+if=|\bchmod\s+\+?s\b|\bsetuid\b|/etc/(passwd|shadow)|169\.254\.169\.254|\bcurl\s+[^|]*\|\s*(sh|bash)|\bwget\s+[^|]*\|\s*(sh|bash)|\bbase64\s+-d[^|]*\|\s*(sh|bash))`)

var injectRe = regexp.MustCompile(`(?i)(ignore\s+(all\s+)?previous|disregard\s+(all\s+)?(previous\s+)?instructions|reveal\s+(your\s+)?(system\s+)?prompt|abaikan\s+(instruksi|perintah)\s+sebelum|bocorkan\s+system\s+prompt|developer\s+mode|do\s+anything\s+now)`)

func Verify(content string) []string {
	var flags []string
	seen := map[string]bool{}
	for _, m := range dangerRe.FindAllString(content, -1) {
		key := "dangerous: " + strings.TrimSpace(m)
		if !seen[key] {
			seen[key] = true
			flags = append(flags, key)
		}
	}
	if m := injectRe.FindString(content); m != "" {
		flags = append(flags, "injection: "+strings.TrimSpace(m))
	}
	return flags
}

func Safe(content string) bool { return len(Verify(content)) == 0 }
