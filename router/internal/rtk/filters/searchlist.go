// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package filters

import (
	"strings"

	"github.com/flowork-os/flowork_Router/internal/rtk"
)

func init() { rtk.Register(&searchList{}) }

type searchList struct{}

var reSearchListHeader = mustCompile(`(?m)^(?:Glob|Search|Files matching) `)

func (s *searchList) Name() string            { return "search-list" }
func (s *searchList) Detect(head string) bool { return reSearchListHeader.MatchString(head) }
func (s *searchList) Apply(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 50 {
		return text
	}
	first := lines[:30]
	last := lines[len(lines)-10:]
	cut := len(lines) - 40
	return strings.Join(first, "\n") +
		"\n…[" + itoa(cut) + " matches trimmed by RTK search-list]…\n" +
		strings.Join(last, "\n")
}
