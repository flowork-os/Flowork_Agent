// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package quotalive

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type Window struct {
	Label            string    `json:"label"`
	Used             float64   `json:"used"`
	Total            float64   `json:"total"`
	Remaining        float64   `json:"remaining"`
	RemainingPercent float64   `json:"remainingPercent"`
	ResetAt          time.Time `json:"resetAt,omitempty"`
	Unlimited        bool      `json:"unlimited,omitempty"`
	Unit             string    `json:"unit,omitempty"`
}

type Snapshot struct {
	Provider  string    `json:"provider"`
	Plan      string    `json:"plan,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
	Windows   []Window  `json:"windows"`
	Raw       []byte    `json:"-"`
}

type Params struct {
	Token      string
	ProviderID string
	Extra      map[string]any
}

type LiveFetcher interface {
	Name() string
	Fetch(ctx context.Context, p Params) (Snapshot, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]LiveFetcher{}
)

func Register(f LiveFetcher) {
	if f == nil || f.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[f.Name()] = f
}

func Get(name string) LiveFetcher {
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

var httpClient = &http.Client{Timeout: 30 * time.Second}
