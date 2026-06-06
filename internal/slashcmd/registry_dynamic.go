// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 16 phase 2 (hot-reload support). API stable: Unregister
//   (remove canonical + alias), Has (existence check). Locked registry.go
//   ngga di-modify — pakai same regMu lewat package access. Phase 3
//   (batch operation, replay journal) → tambah file baru.
//
// registry_dynamic.go — Section 16 phase 2: hot-reload support.

package slashcmd

import "strings"

// Unregister — remove command by canonical name. Also strip aliases yang
// point ke command itu. Idempotent (no-op kalau ngga ada). Return true
// kalau ada yang di-remove.
//
// Caller (custom loader) panggil saat .md di-update / di-delete sebelum
// re-Register dengan body baru.
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
	// Sweep aliases yang target name ini.
	for a, target := range aliasLookup {
		if target == name {
			delete(aliasLookup, a)
		}
	}
	return true
}

// Has — return true kalau name (canonical OR alias) registered.
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
