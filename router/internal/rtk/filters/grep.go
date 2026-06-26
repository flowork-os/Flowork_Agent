// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&grepFilter{}) }

type grepFilter struct{}

var reGrepLine = mustCompile(`^[^:\s]+:\d+:`)

func (g *grepFilter) Name() string { return "grep" }
func (g *grepFilter) Detect(head string) bool {

	lines := strings.Split(head, "\n")
	checked := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if reGrepLine.MatchString(ln) {
			return true
		}
		checked++
		if checked >= 5 {
			break
		}
	}
	return false
}

func (g *grepFilter) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 80 {
		return text
	}
	first := lines[:40]
	last := lines[len(lines)-20:]
	cut := len(lines) - 60
	return strings.Join(first, "\n") +
		"\n…[" + itoa(cut) + " grep results trimmed by RTK]…\n" +
		strings.Join(last, "\n")
}
