// === FROZEN (kernel inti) — DO NOT MODIFY. Kernel FREEZE v1 (2026-06-07). ===
// Owner: Aola Sahidin (Mr.Dev). Bagian microkernel "papan kosong" abadi; checksum
// dipin di KERNEL_FREEZE.md. Ubah = unfreeze eksplisit owner + update manifest.

package loket

import (
	"context"
	"encoding/json"
)

// registryProviders back the discovery capabilities (registry.list /
// registry.providers, §8.D of the contract). A group can find its members, an
// agent can find which module provides a capability — all WITHOUT hardcoding ids,
// so modules stay loosely coupled and plug-and-play. The module list itself is
// host-provided (Deps.Modules); these providers only filter it.
type registryProviders struct {
	deps Deps
}

func (r *registryProviders) all() []ModuleInfo {
	if r.deps.Modules == nil {
		return nil
	}
	return r.deps.Modules()
}

// list returns loaded modules, optionally filtered by kind.
func (r *registryProviders) list(_ context.Context, _ string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(args, &a)
	out := []ModuleInfo{}
	for _, m := range r.all() {
		if a.Kind == "" || m.Kind == a.Kind {
			out = append(out, m)
		}
	}
	return mustJSON(map[string]any{"modules": out, "count": len(out)}), nil
}

// providers returns modules that declare they PROVIDE a given capability.
func (r *registryProviders) providers(_ context.Context, _ string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Cap string `json:"cap"`
	}
	_ = json.Unmarshal(args, &a)
	out := []ModuleInfo{}
	for _, m := range r.all() {
		for _, p := range m.Provides {
			if p == a.Cap {
				out = append(out, m)
				break
			}
		}
	}
	return mustJSON(map[string]any{"modules": out, "count": len(out)}), nil
}
