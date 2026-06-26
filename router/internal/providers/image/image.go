// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package image

import (
	"context"
	"sync"
)

type Request struct {
	Model          string
	Prompt         string
	NegativePrompt string
	Size           string
	N              int
	Quality        string
	Style          string
	APIKey         string
	BaseURL        string
	Extra          map[string]any
}

type Result struct {
	Data []ResultImage `json:"data"`
}

type ResultImage struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

type ImageProvider interface {
	Name() string
	Generate(ctx context.Context, req Request) (*Result, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]ImageProvider{}
)

func Register(p ImageProvider) {
	if p == nil || p.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[p.Name()] = p
}

func Get(name string) ImageProvider {
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
