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

	"github.com/flowork-os/flowork_Router/internal/cloudcode"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&antigravityExecutor{}) }

// AntigravityHeaderHook — SEAM (Pola B, non-frozen ngisi): override/augment header
// yg dikirim ke Google cloudcode-pa dengan header client ASLI hasil capture MITM
// (User-Agent/X-Client-Version/dll) + Bearer terfresh. Default nil → pakai header
// hardcoded (perilaku lama). Plug-and-play: hapus pengisi non-frozen → balik default.
// 📄 Dok: FLowork_os/lock/antigravity.md
var AntigravityHeaderHook func(base map[string]string, p *store.ProviderConnection) map[string]string

type antigravityExecutor struct{}

func (a *antigravityExecutor) Name() string { return "antigravity" }

func (a *antigravityExecutor) endpoint(p *store.ProviderConnection, stream bool) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://cloudcode-pa.googleapis.com"
	}
	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}
	return trimRightSlash(base) + "/v1internal:" + action
}

func (a *antigravityExecutor) headers(p *store.ProviderConnection, stream bool) map[string]string {
	h := map[string]string{
		"User-Agent": "google-cloud-code-assist/1.16.0",
	}
	if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	}

	sid, _ := p.Data["sessionId"].(string)
	if sid == "" {
		sid = DeriveAntigravitySessionID(p.ID)
	}
	h["X-Machine-Session-Id"] = sid
	if stream {
		h["Accept"] = "text/event-stream"
	} else {
		h["Accept"] = "application/json"
	}
	if AntigravityHeaderHook != nil {
		if out := AntigravityHeaderHook(h, p); out != nil {
			return out
		}
	}
	return h
}

func (a *antigravityExecutor) body(ctx context.Context, p *store.ProviderConnection, req Request) []byte {
	contents := make([]map[string]any, len(req.Messages))
	for i, m := range req.Messages {
		contents[i] = map[string]any{"role": m.Role, "parts": []map[string]any{{"text": m.Content}}}
	}
	project := ProviderString(p, "projectId")
	if useReal, _ := p.Data["useRealProjectId"].(bool); useReal {
		if tok, _ := p.Data[store.CfgAPIKey].(string); tok != "" {
			if real, err := cloudcode.GetProjectID(ctx, p.ID, tok); err == nil && real != "" {
				project = real
			}
		}
	}
	wrap := map[string]any{
		"project": project,
		"model":   req.Model,
		"request": map[string]any{"contents": contents},
	}
	b, _ := json.Marshal(wrap)
	return b
}

func (a *antigravityExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, a.endpoint(p, true), a.body(ctx, p, req), a.headers(p, true))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (a *antigravityExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, a.endpoint(p, false), a.body(ctx, p, req), a.headers(p, false))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}
