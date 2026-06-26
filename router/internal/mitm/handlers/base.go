// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package handlers

import (
	"net/http"
	"sync"
)

type Handler interface {
	Name() string
	Handle(w http.ResponseWriter, r *http.Request)
}

var (
	regMu    sync.RWMutex
	registry = map[string]Handler{}
)

func Register(h Handler) {
	if h == nil || h.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[h.Name()] = h
}

func Get(name string) Handler {
	regMu.RLock()
	defer regMu.RUnlock()
	return registry[name]
}

func List() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
