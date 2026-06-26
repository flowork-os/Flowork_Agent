// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package rtk

import (
	"fmt"
	"regexp"
	"sync"
)

type Filter interface {
	Name() string
	Detect(head string) bool
	Apply(text string) string
}

var (
	filtersMu sync.RWMutex
	filters   []Filter
)

func Register(f Filter) {
	filtersMu.Lock()
	defer filtersMu.Unlock()
	filters = append(filters, f)
}

const detectWindow = 8 * 1024

func Compress(text string, cap int) (string, int) {
	if cap <= 0 || len(text) <= cap {
		return text, 0
	}
	head := text
	if len(head) > detectWindow {
		head = head[:detectWindow]
	}

	pick := autoDetect(head)
	var out string
	if pick != nil {
		out = pick.Apply(text)
	} else {
		out = fallbackHeadTail(text, cap)
	}
	if len(out) < len(text) {
		return out, len(text) - len(out)
	}
	return text, 0
}

func pickFilter(head string) Filter {
	filtersMu.RLock()
	defer filtersMu.RUnlock()
	for _, f := range filters {
		if f.Detect(head) {
			return f
		}
	}
	return nil
}

func fallbackHeadTail(s string, cap int) string {
	if len(s) <= cap {
		return s
	}
	headN := cap * 4 / 5
	tailN := cap / 6
	if headN+tailN >= len(s) {
		return s
	}
	cut := len(s) - headN - tailN
	return s[:headN] +
		fmt.Sprintf("\n\n…[%d chars trimmed by RTK]…\n\n", cut) +
		s[len(s)-tailN:]
}

func mustCompile(p string) *regexp.Regexp { return regexp.MustCompile(p) }
