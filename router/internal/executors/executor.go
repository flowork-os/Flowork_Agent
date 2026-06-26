// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"net/http"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Request struct {
	Model       string
	Messages    []Message
	MaxTokens   int
	Temperature float64
	TopP        float64
	Stream      bool
	Tools       []map[string]any
	RawJSON     []byte
}

type Message struct {
	Role    string
	Content string
}

type Executor interface {
	Name() string
	Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error)
	NonStream(ctx context.Context, p *store.ProviderConnection, req Request) (respBody []byte, usage Usage, status int, err error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Executor{}
)

func Register(e Executor) {
	if e == nil || e.Name() == "" {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[e.Name()] = e
}

func Get(name string) Executor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
