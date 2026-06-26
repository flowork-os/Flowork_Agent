// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package slashcmd

import "strings"

func Unregister(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := registry[name]; !ok {
		return false
	}
	delete(registry, name)

	for a, target := range aliasLookup {
		if target == name {
			delete(aliasLookup, a)
		}
	}
	return true
}

func Has(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	regMu.RLock()
	defer regMu.RUnlock()
	if _, ok := registry[name]; ok {
		return true
	}
	_, ok := aliasLookup[name]
	return ok
}
