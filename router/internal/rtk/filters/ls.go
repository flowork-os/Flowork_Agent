// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&lsFilter{}) }

type lsFilter struct{}

var (
	reLsRow   = mustCompile(`(?m)^[-dlbcps][rwx-]{9}`)
	reLsTotal = mustCompile(`(?m)^total \d+$`)
)

func (l *lsFilter) Name() string { return "ls" }
func (l *lsFilter) Detect(head string) bool {
	if reLsTotal.MatchString(head) {
		return true
	}
	return len(reLsRow.FindAllStringIndex(head, -1)) >= 3
}

func (l *lsFilter) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 60 {
		return text
	}
	first := lines[:30]
	last := lines[len(lines)-15:]
	cut := len(lines) - 45
	return strings.Join(first, "\n") +
		"\n…[" + itoa(cut) + " entries trimmed by RTK ls]…\n" +
		strings.Join(last, "\n")
}
