// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&gitDiff{}) }

type gitDiff struct{}

var (
	reGitDiff      = mustCompile(`(?m)^diff --git `)
	reGitDiffHunk  = mustCompile(`(?m)^@@ `)
	reGitDiffStart = mustCompile(`(?m)^(diff --git|index |@@ |\+\+\+ |--- )`)
)

func (g *gitDiff) Name() string { return "git-diff" }
func (g *gitDiff) Detect(head string) bool {
	return reGitDiff.MatchString(head) || reGitDiffHunk.MatchString(head)
}
func (g *gitDiff) Apply(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	dropped := 0
	for _, ln := range lines {

		if strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "  ") {
			dropped++
			continue
		}
		out = append(out, ln)
	}
	res := strings.Join(out, "\n")
	if dropped > 0 {
		res += "\n\n…[" + itoa(dropped) + " context lines trimmed by RTK git-diff]…"
	}
	return res
}
