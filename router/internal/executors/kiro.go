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

func init() { Register(&kiroExecutor{}) }

type kiroExecutor struct{}

func (k *kiroExecutor) Name() string { return "kiro" }

func (k *kiroExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://codewhisperer.us-east-1.amazonaws.com"
	}
	return trimRightSlash(base) + "/generateAssistantResponse"
}

func (k *kiroExecutor) headers(p *store.ProviderConnection) map[string]string {
	h := map[string]string{
		"Accept":                "text/event-stream",
		"amz-sdk-invocation-id": "flow-router",
	}
	if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	if pid, ok := p.Data["profileArn"].(string); ok && pid != "" {
		h["x-amzn-codewhisperer-profile-arn"] = pid
	}
	return h
}

func kiroBody(r Request) []byte {
	isAgentic := IsKiroAgenticModel(r.Model)
	resolvedModel := ResolveKiroModel(r.Model)

	var history []map[string]any
	var current map[string]any

	if isAgentic {
		history = append(history, map[string]any{
			"role": "system",
			"content": []map[string]any{
				{"type": "text", "text": KiroAgenticSystemPrompt},
			},
		})
	}

	for i, m := range r.Messages {
		entry := map[string]any{"role": m.Role, "content": []map[string]any{{"type": "text", "text": m.Content}}}
		if i == len(r.Messages)-1 && m.Role == "user" {
			current = entry
			continue
		}
		history = append(history, entry)
	}
	body, _ := json.Marshal(map[string]any{
		"conversationState": map[string]any{
			"chatTriggerType": "MANUAL",
			"history":         history,
			"currentMessage":  current,
			"modelId":         resolvedModel,
		},
	})
	return body
}

func (k *kiroExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, k.endpoint(p), kiroBody(req), k.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (k *kiroExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, k.endpoint(p), kiroBody(req), k.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}
