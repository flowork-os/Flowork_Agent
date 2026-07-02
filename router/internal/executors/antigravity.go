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
	"sync/atomic"

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
	// Spec Antigravity (opencode-antigravity-auth ANTIGRAVITY_API_SPEC.md): body
	// WAJIB bawa userAgent + requestId (tanpa itu cloudcode-pa balik 404 routing).
	wrap := map[string]any{
		"project":   project,
		"model":     req.Model,
		"request":   map[string]any{"contents": contents},
		"userAgent": "antigravity",
		"requestId": fmt.Sprintf("flowork-%d", atomic.AddUint64(&antigravityReqSeq, 1)),
	}
	b, _ := json.Marshal(wrap)
	return b
}

var antigravityReqSeq uint64

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
	body, u, st, err := DoNonStream(httpReq)
	if err != nil || st < 200 || st >= 300 {
		return body, u, st, err
	}
	// AKAR "(no choices)": cloudcode-pa balik {"response":{"candidates":[{content:
	// {parts:[{text}]}}]}} (Gemini nested). Terjemah ke OpenAI {choices:[{message}]}
	// biar dispatcher (unmarshal OpenAIResponse) dapet isinya.
	if oai := antigravityRespToOpenAI(body, req.Model); oai != nil {
		return oai, u, st, nil
	}
	return body, u, st, err
}

// antigravityRespToOpenAI — konversi respons cloudcode-pa (Gemini nested) → OpenAI.
// nil kalau ga bisa parse (biar fallback ke body asli).
func antigravityRespToOpenAI(body []byte, model string) []byte {
	var parsed struct {
		Response struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
				TotalTokenCount      int `json:"totalTokenCount"`
			} `json:"usageMetadata"`
		} `json:"response"`
	}
	if json.Unmarshal(body, &parsed) != nil || len(parsed.Response.Candidates) == 0 {
		return nil
	}
	var text string
	for _, pt := range parsed.Response.Candidates[0].Content.Parts {
		text += pt.Text
	}
	finish := "stop"
	switch parsed.Response.Candidates[0].FinishReason {
	case "MAX_TOKENS":
		finish = "length"
	case "SAFETY":
		finish = "content_filter"
	}
	out := map[string]any{
		"id":      "chatcmpl-antigravity",
		"object":  "chat.completion",
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": finish,
		}},
		"usage": map[string]any{
			"prompt_tokens":     parsed.Response.UsageMetadata.PromptTokenCount,
			"completion_tokens": parsed.Response.UsageMetadata.CandidatesTokenCount,
			"total_tokens":      parsed.Response.UsageMetadata.TotalTokenCount,
		},
	}
	b, _ := json.Marshal(out)
	return b
}
