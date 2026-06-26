// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"context"
	"fmt"
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() {
	Register(&cursorExecutor{name: "cursor"})
	Register(&cursorExecutor{name: "cu"})
}

type cursorExecutor struct{ name string }

func (c *cursorExecutor) Name() string { return c.name }

func (c *cursorExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://api2.cursor.sh"
	}
	return trimRightSlash(base) + "/aiserver.v1.ChatService/StreamChat"
}

func (c *cursorExecutor) headers(p *store.ProviderConnection) map[string]string {
	tok, _ := p.Data[store.CfgAPIKey].(string)
	machineID, _ := p.Data["machineId"].(string)
	storedChecksum, _ := p.Data["cursorChecksum"].(string)
	storedSession, _ := p.Data["sessionId"].(string)

	if tok != "" && storedChecksum == "" {
		h := BuildCursorHeaders(tok, machineID, false)
		h["Accept"] = "*/*"
		if storedSession != "" {
			h["x-cursor-session-id"] = storedSession
		}
		return h
	}

	h := map[string]string{
		"Accept":       "*/*",
		"x-ghost-mode": "false",
		"x-client-key": "upstream",
	}
	if tok != "" {
		h["Authorization"] = "Bearer " + tok
	}
	if storedChecksum != "" {
		h["x-cursor-checksum"] = storedChecksum
	}
	if storedSession != "" {
		h["x-cursor-session-id"] = storedSession
	}
	return h
}

func (c *cursorExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {
	body := MarshalRequest(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, c.endpoint(p), body, c.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoStream(httpReq, w, flusher)
}

func (c *cursorExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	req.Stream = false
	body := MarshalRequest(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, c.endpoint(p), body, c.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	return DoNonStream(httpReq)
}
