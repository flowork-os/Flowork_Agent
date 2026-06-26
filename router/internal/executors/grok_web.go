// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&grokWebExecutor{}) }

type grokWebExecutor struct{}

func (g *grokWebExecutor) Name() string { return "grok-web" }

var grokModelMap = map[string]struct {
	GrokModel  string
	ModelMode  string
	IsThinking bool
}{
	"grok-3":            {"grok-3", "MODEL_MODE_GROK_3", false},
	"grok-3-mini":       {"grok-3", "MODEL_MODE_GROK_3_MINI_THINKING", true},
	"grok-3-thinking":   {"grok-3", "MODEL_MODE_GROK_3_THINKING", true},
	"grok-4":            {"grok-4", "MODEL_MODE_GROK_4", false},
	"grok-4-mini":       {"grok-4-mini", "MODEL_MODE_GROK_4_MINI_THINKING", true},
	"grok-4-thinking":   {"grok-4", "MODEL_MODE_GROK_4_THINKING", true},
	"grok-4-heavy":      {"grok-4", "MODEL_MODE_HEAVY", true},
	"grok-4.1-mini":     {"grok-4-1-thinking-1129", "MODEL_MODE_GROK_4_1_MINI_THINKING", true},
	"grok-4.1-fast":     {"grok-4-1-thinking-1129", "MODEL_MODE_FAST", false},
	"grok-4.1-expert":   {"grok-4-1-thinking-1129", "MODEL_MODE_EXPERT", true},
	"grok-4.1-thinking": {"grok-4-1-thinking-1129", "MODEL_MODE_GROK_4_1_THINKING", true},
	"grok-4.2":          {"grok-420", "MODEL_MODE_GROK_420", false},
	"grok-4.20":         {"grok-420", "MODEL_MODE_GROK_420", false},
}

func (g *grokWebExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://grok.com/rest/app-chat/conversations/new"
	}
	return base
}

func (g *grokWebExecutor) headers(p *store.ProviderConnection) map[string]string {
	h := map[string]string{
		"User-Agent":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
		"Content-Type": "application/json",
	}
	if cookie, ok := p.Data["cookie"].(string); ok && cookie != "" {
		h["Cookie"] = cookie
	}
	return h
}

func (g *grokWebExecutor) body(req Request) []byte {
	m, ok := grokModelMap[req.Model]
	if !ok {
		m = grokModelMap["grok-3"]
	}

	prompt := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" || msg.Role == "system" {
			prompt += msg.Content + "\n"
		}
	}
	out := map[string]any{
		"message":               prompt,
		"modelName":             m.GrokModel,
		"modelMode":             m.ModelMode,
		"isPreset":              false,
		"isReasoning":           m.IsThinking,
		"returnImageBytes":      false,
		"returnRawGrokResponse": false,
	}
	b, _ := json.Marshal(out)
	return b
}

func (g *grokWebExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, g.endpoint(p), g.body(req), g.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (g *grokWebExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, g.endpoint(p), g.body(req), g.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}
