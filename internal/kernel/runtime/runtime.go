// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Wazero engine wrapper. Audit pass — mu protects instances map,
//   Close() Unlock sebelum engine close (anti-deadlock), Unload+Close
//   resource-safe.
//
// Package runtime — wazero wrapper. Phase 5 menambah real instantiate +
// host module bindings. Pattern: kernel construct sekali, panggil
// Bootstrap untuk WASI + flowork host imports, lalu LoadInstance per
// plugin.

package runtime

import (
	"context"
	"errors"
	"sync"

	"github.com/tetratelabs/wazero"
)

// Runtime owns the wazero engine + map plugin instance.
type Runtime struct {
	rt        wazero.Runtime
	mu        sync.Mutex
	instances map[string]*Instance
}

// New construct runtime kosong. Caller wajib panggil Bootstrap sebelum
// LoadInstance.
func New(ctx context.Context) *Runtime {
	rt := wazero.NewRuntime(ctx)
	return &Runtime{
		rt:        rt,
		instances: map[string]*Instance{},
	}
}

// Unload remove plugin dari registry + tutup compiled module-nya.
func (r *Runtime) Unload(ctx context.Context, pluginID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.instances[pluginID]
	if !ok {
		return nil
	}
	delete(r.instances, pluginID)
	if inst.compiled != nil {
		return inst.compiled.Close(ctx)
	}
	return nil
}

// Loaded returns plugin id yang sedang ter-instantiate.
func (r *Runtime) Loaded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.instances))
	for id := range r.instances {
		out = append(out, id)
	}
	return out
}

// Close lepas semua plugin + tutup engine.
func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	for id, inst := range r.instances {
		if inst.compiled != nil {
			_ = inst.compiled.Close(ctx)
		}
		delete(r.instances, id)
	}
	r.mu.Unlock()
	if r.rt != nil {
		return r.rt.Close(ctx)
	}
	return nil
}

// ErrAlreadyLoaded — return kalau LoadInstance dipanggil dua kali untuk
// pluginID yang sama. Caller harus Unload dulu.
var ErrAlreadyLoaded = errors.New("plugin already loaded")
