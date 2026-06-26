// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"fmt"
	"strings"
)

const ftsTable = "memory_fts"

func ftsTokens(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var parts []string
	for _, f := range strings.Fields(q) {
		var b strings.Builder
		for _, r := range f {
			switch r {
			case '"', '\'', '?', '.', ',', ':', ';', '!', '(', ')', '[', ']', '{', '}',
				'*', '/', '\\', '|', '&', '#', '@', '+', '=', '<', '>', '`', '~':
				continue
			default:
				b.WriteRune(r)
			}
		}
		clean := b.String()
		if len(clean) < 2 {
			continue
		}
		parts = append(parts, fmt.Sprintf(`"%s"`, clean))
	}
	return parts
}

func joinFTS(tokens []string, op string) string {
	return strings.Join(tokens, " "+op+" ")
}
