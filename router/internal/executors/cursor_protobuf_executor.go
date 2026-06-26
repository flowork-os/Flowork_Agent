// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() { Register(&cursorProtoExecutor{}) }

type cursorProtoExecutor struct{}

func (c *cursorProtoExecutor) Name() string { return "cursor-proto" }

func (c *cursorProtoExecutor) endpoint(p *store.ProviderConnection) string {
	base := ProviderString(p, store.CfgBaseURL)
	if base == "" {
		base = "https://api2.cursor.sh"
	}
	return trimRightSlash(base) + "/aiserver.v1.ChatService/StreamUnifiedChat"
}

func (c *cursorProtoExecutor) headers(p *store.ProviderConnection) map[string]string {
	tok, _ := p.Data[store.CfgAPIKey].(string)
	machineID, _ := p.Data["machineId"].(string)
	storedChecksum, _ := p.Data["cursorChecksum"].(string)
	storedSession, _ := p.Data["sessionId"].(string)

	if tok != "" && storedChecksum == "" {
		h := BuildCursorHeaders(tok, machineID, false)

		h["Connect-Protocol"] = "1"
		if storedSession != "" {
			h["x-cursor-session-id"] = storedSession
		}
		return h
	}

	h := map[string]string{
		"Content-Type":     "application/connect+proto",
		"Connect-Protocol": "1",
		"x-ghost-mode":     "false",
		"x-client-key":     "flow-router",
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

func (c *cursorProtoExecutor) buildProtoBody(req Request) []byte {
	msgs := make([]CursorMessage, 0, len(req.Messages))
	for _, m := range req.Messages {

		role := m.Role
		if role == "system" {
			role = "user"
		}
		msgs = append(msgs, CursorMessage{Content: m.Content, Role: role})
	}
	model := req.Model
	if model == "" {
		model = "claude-3.5-sonnet"
	}
	encoded := encodeCursorChatRequest(msgs, model)
	return wrapConnectRPCFrame(encoded)
}

func (c *cursorProtoExecutor) Stream(ctx context.Context, p *store.ProviderConnection, req Request, w http.ResponseWriter, flusher http.Flusher) (Usage, int, error) {

	body := c.buildProtoBody(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, c.endpoint(p), body, c.headers(p))
	if err != nil {
		return Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return Usage{}, 0, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {

		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Usage{}, resp.StatusCode, fmt.Errorf("cursor proto %d: %s", resp.StatusCode, string(errBody))
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	emittedID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	for {
		n, readErr := resp.Body.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			for {
				frame := parseConnectRPCFrame(buf.Bytes())
				if frame == nil {
					break
				}

				rest := buf.Bytes()[frame.Consumed:]
				buf.Reset()
				buf.Write(rest)

				text := extractTextFromCursorResponse(frame.Payload)
				if text != "" {
					emitOpenAIDelta(w, flusher, emittedID, req.Model, text)
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	emitOpenAIDone(w, flusher, emittedID, req.Model)
	return Usage{}, http.StatusOK, nil
}

func (c *cursorProtoExecutor) NonStream(ctx context.Context, p *store.ProviderConnection, req Request) ([]byte, Usage, int, error) {
	body := c.buildProtoBody(req)
	httpReq, err := BuildRequest(ctx, http.MethodPost, c.endpoint(p), body, c.headers(p))
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("build req: %w", err)
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, Usage{}, 0, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, Usage{}, resp.StatusCode, fmt.Errorf("cursor proto %d: %s", resp.StatusCode, string(errBody))
	}
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))

	var text bytes.Buffer
	offset := 0
	for offset < len(rawBody) {
		frame := parseConnectRPCFrame(rawBody[offset:])
		if frame == nil {
			break
		}
		text.WriteString(extractTextFromCursorResponse(frame.Payload))
		offset += frame.Consumed
	}

	openAIResp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text.String()},
			"finish_reason": "stop",
		}},
	}
	out, _ := json.Marshal(openAIResp)
	return out, Usage{}, http.StatusOK, nil
}

func emitOpenAIDelta(w http.ResponseWriter, flusher http.Flusher, id, model, text string) {
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{"content": text},
		}},
	}
	raw, _ := json.Marshal(chunk)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()
}

func emitOpenAIDone(w http.ResponseWriter, flusher http.Flusher, id, model string) {
	stop := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": "stop",
		}},
	}
	raw, _ := json.Marshal(stop)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte("\n\ndata: [DONE]\n\n"))
	flusher.Flush()
}
