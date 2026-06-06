// dynamic.go — runtime (de)registration buat PLUG-AND-PLAY tool-pack.
//
// registry.go LOCKED (frozen post-init, panic-on-dup). File ini NAMBAH (sesuai
// arahan registry.go: "Phase 2 → tambah file baru, JANGAN modify ini"):
//   - RegisterDynamic: register/replace tool plugin runtime (anti panic).
//   - Unregister: cabut tool plugin (TOLAK kalau builtin — safety).
//   - IsDynamic / DynamicNames: tracking mana tool plugin vs builtin.
//
// Tool builtin (init() Register) TIDAK pernah bisa di-unregister lewat sini —
// `dynamicNames` cuma nandain yang masuk via RegisterDynamic. Thread-safe pakai
// regMu yang sama (singleton registry).

package tools

import (
	"fmt"
	"sort"
)

// dynamicNames — set nama tool yang di-register runtime (plugin). Builtin NGGAK
// masuk sini → ga bisa di-Unregister (proteksi inti).
var dynamicNames = map[string]bool{}

// RegisterDynamic — register/replace tool plugin (runtime). Beda dari Register
// (panic on dup): ini overwrite kalau sudah ada DAN dia dynamic. TOLAK kalau
// bentrok nama builtin (ga boleh nimpa inti).
func RegisterDynamic(t Tool) error {
	if t == nil {
		return fmt.Errorf("RegisterDynamic: nil tool")
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("RegisterDynamic: empty name")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := registry[name]; ok && !dynamicNames[name] {
		return fmt.Errorf("nama %q bentrok tool builtin (ga boleh ditimpa)", name)
	}
	registry[name] = t
	dynamicNames[name] = true
	return nil
}

// Unregister — cabut tool PLUGIN. TOLAK kalau builtin atau ga ada.
func Unregister(name string) error {
	regMu.Lock()
	defer regMu.Unlock()
	if !dynamicNames[name] {
		return fmt.Errorf("tool %q bukan plugin (atau ga ada) — ga bisa di-uninstall", name)
	}
	delete(registry, name)
	delete(dynamicNames, name)
	return nil
}

// IsDynamic — true kalau tool plugin (bukan builtin).
func IsDynamic(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	return dynamicNames[name]
}

// IsBuiltinName — true kalau name = tool BUILTIN (registered tapi bukan plugin).
// Dipakai install tool-pack: TOLAK kalau bentrok builtin.
func IsBuiltinName(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	_, ok := registry[name]
	return ok && !dynamicNames[name]
}

// DynamicNames — sorted list nama tool plugin (buat GUI "installed tools").
func DynamicNames() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(dynamicNames))
	for n := range dynamicNames {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
