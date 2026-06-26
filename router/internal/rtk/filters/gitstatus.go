// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&gitStatus{}) }

type gitStatus struct{}

var (
	reGitStatusHead = mustCompile(`(?m)^On branch |^nothing to commit|^Changes (not |to be )|^Untracked files:`)
	rePorcelainLine = mustCompile(`(?m)^[ MADRCU?!][ MADRCU?!] \S`)
)

func (g *gitStatus) Name() string { return "git-status" }
func (g *gitStatus) Detect(head string) bool {
	return reGitStatusHead.MatchString(head) || isMostlyPorcelain(head)
}

func (g *gitStatus) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) < 30 {
		return text
	}

	first := lines[:20]
	last := lines[len(lines)-10:]
	cut := len(lines) - 30
	return strings.Join(first, "\n") +
		"\n…[" + itoa(cut) + " path lines trimmed by RTK git-status]…\n" +
		strings.Join(last, "\n")
}

func isMostlyPorcelain(s string) bool {
	if len(s) < 32 {
		return false
	}
	matches := rePorcelainLine.FindAllStringIndex(s, -1)
	if len(matches) < 3 {
		return false
	}
	lines := strings.Count(s, "\n") + 1
	return float64(len(matches))/float64(lines) > 0.6
}
