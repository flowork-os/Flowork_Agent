// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&readNumbered{}) }

type readNumbered struct{}

var reReadNumberedLine = mustCompile(`(?m)^\s*\d+\s*[|\t]`)

func (r *readNumbered) Name() string { return "read-numbered" }
func (r *readNumbered) Detect(head string) bool {
	lines := strings.Split(head, "\n")
	if len(lines) < 8 {
		return false
	}
	hits := 0
	for _, ln := range lines {
		if reReadNumberedLine.MatchString(ln) {
			hits++
		}
	}

	return float64(hits)/float64(len(lines)) > 0.5
}

func (r *readNumbered) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 100 {
		return text
	}
	first := lines[:60]
	last := lines[len(lines)-30:]
	cut := len(lines) - 90
	return strings.Join(first, "\n") +
		"\n…[" + itoa(cut) + " lines trimmed by RTK read-numbered]…\n" +
		strings.Join(last, "\n")
}
