// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&dedupLog{}) }

type dedupLog struct{}

func (d *dedupLog) Name() string { return "dedup-log" }
func (d *dedupLog) Detect(head string) bool {

	lines := strings.Split(head, "\n")
	if len(lines) < 8 {
		return false
	}
	run := 1
	for i := 1; i < len(lines); i++ {
		if lines[i] == lines[i-1] {
			run++
			if run >= 4 {
				return true
			}
		} else {
			run = 1
		}
	}
	return false
}

func (d *dedupLog) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		return text
	}
	var out []string
	i := 0
	saved := 0
	for i < len(lines) {
		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		run := j - i
		if run >= 3 {
			out = append(out, lines[i]+"  …×"+itoa(run))
			saved += run - 1
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	res := strings.Join(out, "\n")
	if saved > 0 {
		res += "\n…[" + itoa(saved) + " duplicate lines collapsed by RTK dedup-log]…"
	}
	return res
}
