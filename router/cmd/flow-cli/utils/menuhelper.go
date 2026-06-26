// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package utils

import "fmt"

type MenuItem struct {
	Label  string
	Action func() error
}

func RunMenu(title string, items []MenuItem) error {
	for {
		Header(title)
		labels := make([]string, len(items)+1)
		for i, it := range items {
			labels[i] = it.Label
		}
		labels[len(items)] = "Back"
		idx := Select("Choose", labels)
		if idx < 0 || idx == len(items) {
			return nil
		}
		if err := items[idx].Action(); err != nil {
			Error(fmt.Sprintf("%v", err))
		}
	}
}
