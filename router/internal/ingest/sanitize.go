// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package ingest

import (
	"strings"
	"unicode"
)

const MaxSanitizeBytes = 256 * 1024

func Sanitize(s string) string {
	if s == "" {
		return ""
	}

	if len(s) > MaxSanitizeBytes {
		s = s[:MaxSanitizeBytes]
	}

	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()

	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}

	for strings.Contains(s, "   ") {
		s = strings.ReplaceAll(s, "   ", "  ")
	}

	return strings.TrimSpace(s)
}
