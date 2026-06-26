// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&treeFilter{}) }

type treeFilter struct{}

var reTreeGlyph = mustCompile(`[├└]──|│  `)

func (t *treeFilter) Name() string            { return "tree" }
func (t *treeFilter) Detect(head string) bool { return reTreeGlyph.MatchString(head) }
func (t *treeFilter) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 80 {
		return text
	}
	first := lines[:50]
	last := lines[len(lines)-20:]
	cut := len(lines) - 70
	return strings.Join(first, "\n") +
		"\n…[" + itoa(cut) + " tree entries trimmed by RTK]…\n" +
		strings.Join(last, "\n")
}
