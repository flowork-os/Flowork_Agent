// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"fmt"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&smartTruncate{}) }

type smartTruncate struct{}

func (s *smartTruncate) Name() string            { return "smart-truncate" }
func (s *smartTruncate) Detect(head string) bool { return false }
func (s *smartTruncate) Apply(text string) string {
	const cap = 4000
	if len(text) <= cap {
		return text
	}
	headN := cap * 4 / 5
	tailN := cap / 6
	cut := len(text) - headN - tailN
	return text[:headN] +
		fmt.Sprintf("\n\n…[%d chars trimmed by RTK smart-truncate]…\n\n", cut) +
		text[len(text)-tailN:]
}

var _ = strings.Builder{}
