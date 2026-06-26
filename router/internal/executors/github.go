// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&githubExecutor{}) }

type githubExecutor struct{}

func (g *githubExecutor) Name() string { return "github" }

func (g *githubExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://api.githubcopilot.com"
	}
	return trimRightSlash(base) + "/chat/completions"
}

func (g *githubExecutor) headers(p *store.ProviderConnection, stream bool) map[string]string {
	h := map[string]string{
		"copilot-integration-id":              "vscode-chat",
		"editor-version":                      "vscode/1.99.0",
		"editor-plugin-version":               "copilot-chat/0.20.0",
		"user-agent":                          "GitHubCopilotChat/0.20.0",
		"openai-intent":                       "conversation-panel",
		"x-github-api-version":                "2024-12-15",
		"x-request-id":                        randomGitHubReqID(),
		"x-vscode-user-agent-library-version": "electron-fetch",
		"X-Initiator":                         "user",
	}
	if tok, ok := p.Data["copilotToken"].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	} else if tok, ok := p.Data[store.CfgAPIKey].(string); ok && tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	if stream {
		h["Accept"] = "text/event-stream"
	} else {
		h["Accept"] = "application/json"
	}
	return h
}

func randomGitHubReqID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (g *githubExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	httpReq, err := BuildRequest(ctx, http.MethodPost, g.endpoint(p), MarshalRequest(req), g.headers(p, true))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (g *githubExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	httpReq, err := BuildRequest(ctx, http.MethodPost, g.endpoint(p), MarshalRequest(req), g.headers(p, false))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}
