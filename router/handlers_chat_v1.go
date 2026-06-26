// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI (multi-tab): peta dok di lock/gui/README.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/store"
)

type anthropicMessagesRequest struct {
	Model       string           `json:"model"`
	Messages    []anthropicMsgIn `json:"messages"`
	System      json.RawMessage  `json:"system,omitempty"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature float64          `json:"temperature,omitempty"`
	TopP        float64          `json:"top_p,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

type anthropicMsgIn struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func messagesV1Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var anth anthropicMessagesRequest
	if err := json.Unmarshal(raw, &anth); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if anth.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}
	if anth.MaxTokens <= 0 {
		anth.MaxTokens = 4096
	}

	openaiReq := router.OpenAIRequest{
		Model:       anth.Model,
		MaxTokens:   anth.MaxTokens,
		Temperature: anth.Temperature,
		TopP:        anth.TopP,
		Stream:      anth.Stream,
	}
	if sys := flattenAnthropicSystem(anth.System); sys != "" {
		openaiReq.Messages = append(openaiReq.Messages, router.OpenAIMessage{Role: "system", Content: sys})
	}
	for _, m := range anth.Messages {
		openaiReq.Messages = append(openaiReq.Messages, router.OpenAIMessage{
			Role:    m.Role,
			Content: flattenAnthropicContent(m.Content),
		})
	}

	if anth.Stream {

		streamAsAnthropicSSE(w, r, openaiReq)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	resp, status, err := router.DispatchChatCompletion(ctx, openaiReq)
	if err != nil {
		writeJSON(w, status, map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "router_error",
				"message": err.Error(),
			},
		})
		return
	}

	out := openaiToAnthropicResp(resp)
	writeJSON(w, status, out)
}

func flattenAnthropicSystem(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, p := range arr {
			if p.Type == "text" {
				parts = append(parts, p.Text)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	return ""
}

func flattenAnthropicContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, p := range arr {
			if p.Type == "text" {
				parts = append(parts, p.Text)
			}
		}
		return strings.Join(parts, "")
	}
	return string(raw)
}

func openaiToAnthropicResp(r *router.OpenAIResponse) map[string]any {
	var content string
	stop := "end_turn"
	if len(r.Choices) > 0 {
		content = r.Choices[0].Message.Content
		switch r.Choices[0].FinishReason {
		case "length":
			stop = "max_tokens"
		case "tool_calls":
			stop = "tool_use"
		default:
			stop = "end_turn"
		}
	}
	return map[string]any{
		"id":          r.ID,
		"type":        "message",
		"role":        "assistant",
		"model":       r.Model,
		"content":     []map[string]any{{"type": "text", "text": content}},
		"stop_reason": stop,
		"usage": map[string]any{
			"input_tokens":  r.Usage.PromptTokens,
			"output_tokens": r.Usage.CompletionTokens,
		},
	}
}

func streamAsAnthropicSSE(w http.ResponseWriter, r *http.Request, req router.OpenAIRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()

		_, _, _ = router.DispatchChatCompletionStream(r.Context(), req, &pipeWriter{pw: pw})
	}()

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	emitAnthropicEvent(w, flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      msgID,
			"type":    "message",
			"role":    "assistant",
			"content": []any{},
			"model":   req.Model,
			"usage":   map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
	emitAnthropicEvent(w, flusher, "content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})

	scanner := newSSELineScanner(pr)
	var totalOut int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" || payload == "" {
			continue
		}
		var probe struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				CompletionTokens int `json:"completion_tokens"`
				PromptTokens     int `json:"prompt_tokens"`
			} `json:"usage"`
		}
		if jerr := json.Unmarshal([]byte(payload), &probe); jerr != nil {
			continue
		}
		for _, c := range probe.Choices {
			if c.Delta.Content != "" {
				emitAnthropicEvent(w, flusher, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]any{"type": "text_delta", "text": c.Delta.Content},
				})
				totalOut += len(c.Delta.Content) / 4
			}
		}
	}
	emitAnthropicEvent(w, flusher, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	emitAnthropicEvent(w, flusher, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": totalOut},
	})
	emitAnthropicEvent(w, flusher, "message_stop", map[string]any{"type": "message_stop"})
}

func emitAnthropicEvent(w http.ResponseWriter, flusher http.Flusher, eventName string, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, string(raw))
	flusher.Flush()
}

type pipeWriter struct {
	pw     *io.PipeWriter
	header http.Header
	status int
}

func (p *pipeWriter) Header() http.Header {
	if p.header == nil {
		p.header = http.Header{}
	}
	return p.header
}

func (p *pipeWriter) Write(b []byte) (int, error) { return p.pw.Write(b) }
func (p *pipeWriter) WriteHeader(code int)        { p.status = code }
func (p *pipeWriter) Flush()                      {}

type sseLineScanner struct {
	sc *bufio.Scanner
}

func newSSELineScanner(r io.Reader) *sseLineScanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	return &sseLineScanner{sc: s}
}

func (s *sseLineScanner) Scan() bool   { return s.sc.Scan() }
func (s *sseLineScanner) Text() string { return s.sc.Text() }

type openAIResponsesRequest struct {
	Model        string          `json:"model"`
	Input        json.RawMessage `json:"input"`
	Instructions string          `json:"instructions,omitempty"`
	Temperature  float64         `json:"temperature,omitempty"`
	MaxTokens    int             `json:"max_output_tokens,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
}

func responsesV1Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var rq openAIResponsesRequest
	if err := json.NewDecoder(r.Body).Decode(&rq); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if rq.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}

	openaiReq := router.OpenAIRequest{
		Model:       rq.Model,
		Temperature: rq.Temperature,
		MaxTokens:   rq.MaxTokens,
		Stream:      rq.Stream,
	}
	if rq.Instructions != "" {
		openaiReq.Messages = append(openaiReq.Messages, router.OpenAIMessage{Role: "system", Content: rq.Instructions})
	}
	openaiReq.Messages = append(openaiReq.Messages, parseResponsesInput(rq.Input)...)
	if openaiReq.MaxTokens <= 0 {
		openaiReq.MaxTokens = 4096
	}

	if rq.Stream {

		streamAsResponsesSSE(w, r, openaiReq)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	resp, status, err := router.DispatchChatCompletion(ctx, openaiReq)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": err.Error(), "type": "router_error"}})
		return
	}
	var content string
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	out := map[string]any{
		"id":         resp.ID,
		"object":     "response",
		"created_at": resp.Created,
		"model":      resp.Model,
		"status":     "completed",
		"output": []map[string]any{
			{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": content},
				},
			},
		},
		"usage": map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.TotalTokens,
		},
	}
	writeJSON(w, status, out)
}

func parseResponsesInput(raw json.RawMessage) []router.OpenAIMessage {
	if len(raw) == 0 {
		return nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []router.OpenAIMessage{{Role: "user", Content: s}}
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return []router.OpenAIMessage{{Role: "user", Content: string(raw)}}
	}
	var out []router.OpenAIMessage
	for _, item := range arr {

		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			out = append(out, router.OpenAIMessage{Role: "user", Content: s})
			continue
		}
		var obj struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(item, &obj); err == nil {
			role := obj.Role
			if role == "" {
				role = "user"
			}
			out = append(out, router.OpenAIMessage{Role: role, Content: flattenAnthropicContent(obj.Content)})
		}
	}
	return out
}

func streamAsResponsesSSE(w http.ResponseWriter, r *http.Request, req router.OpenAIRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_, _, _ = router.DispatchChatCompletionStream(r.Context(), req, &pipeWriter{pw: pw})
	}()
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	emitResponsesEvent(w, flusher, "response.created", map[string]any{
		"type":     "response.created",
		"response": map[string]any{"id": respID, "object": "response", "model": req.Model, "status": "in_progress"},
	})

	scanner := newSSELineScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" || payload == "" {
			continue
		}
		var probe struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &probe) != nil {
			continue
		}
		for _, c := range probe.Choices {
			if c.Delta.Content != "" {
				emitResponsesEvent(w, flusher, "response.output_text.delta", map[string]any{
					"type":  "response.output_text.delta",
					"delta": c.Delta.Content,
				})
			}
		}
	}
	emitResponsesEvent(w, flusher, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": map[string]any{"id": respID, "object": "response", "model": req.Model, "status": "completed"},
	})
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func emitResponsesEvent(w http.ResponseWriter, flusher http.Flusher, name string, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, string(raw))
	flusher.Flush()
}

func v1betaModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	providers, _ := store.ListProviders(d)
	seen := map[string]bool{}
	var models []map[string]any
	for _, p := range providers {
		if !p.IsActive {
			continue
		}
		ms, _ := p.Data[store.CfgModels].([]any)
		for _, m := range ms {
			s, ok := m.(string)
			if !ok || s == "" || s == "*" {
				continue
			}
			if seen[s] {
				continue
			}
			seen[s] = true
			models = append(models, map[string]any{
				"name":                       "models/" + s,
				"displayName":                s,
				"description":                fmt.Sprintf("Served via %s", p.Name),
				"supportedGenerationMethods": []string{"generateContent", "streamGenerateContent"},
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func v1betaGenerateContentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	parts := strings.SplitN(path, ":", 2)
	if len(parts) != 2 {
		http.Error(w, "expected /v1beta/models/<model>:generateContent", http.StatusBadRequest)
		return
	}
	model := parts[0]
	action := parts[1]
	stream := strings.HasPrefix(action, "stream")

	var body struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		SystemInstruction *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction,omitempty"`
		GenerationConfig *struct {
			Temperature     float64 `json:"temperature,omitempty"`
			MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
			TopP            float64 `json:"topP,omitempty"`
		} `json:"generationConfig,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	openaiReq := router.OpenAIRequest{Model: model, Stream: stream}
	if body.GenerationConfig != nil {
		openaiReq.Temperature = body.GenerationConfig.Temperature
		openaiReq.MaxTokens = body.GenerationConfig.MaxOutputTokens
		openaiReq.TopP = body.GenerationConfig.TopP
	}
	if body.SystemInstruction != nil {
		var sysText strings.Builder
		for _, p := range body.SystemInstruction.Parts {
			sysText.WriteString(p.Text)
		}
		if sysText.Len() > 0 {
			openaiReq.Messages = append(openaiReq.Messages, router.OpenAIMessage{Role: "system", Content: sysText.String()})
		}
	}
	for _, c := range body.Contents {
		role := c.Role
		if role == "model" {
			role = "assistant"
		}
		if role == "" {
			role = "user"
		}
		var txt strings.Builder
		for _, p := range c.Parts {
			txt.WriteString(p.Text)
		}
		openaiReq.Messages = append(openaiReq.Messages, router.OpenAIMessage{Role: role, Content: txt.String()})
	}
	if openaiReq.MaxTokens <= 0 {
		openaiReq.MaxTokens = 4096
	}

	if stream {
		streamAsGeminiSSE(w, r, openaiReq)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	resp, status, err := router.DispatchChatCompletion(ctx, openaiReq)
	if err != nil {
		writeJSON(w, status, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	var content string
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	out := map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": content}},
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     resp.Usage.PromptTokens,
			"candidatesTokenCount": resp.Usage.CompletionTokens,
			"totalTokenCount":      resp.Usage.TotalTokens,
		},
		"modelVersion": resp.Model,
	}
	writeJSON(w, status, out)
}

func streamAsGeminiSSE(w http.ResponseWriter, r *http.Request, req router.OpenAIRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_, _, _ = router.DispatchChatCompletionStream(r.Context(), req, &pipeWriter{pw: pw})
	}()
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	scanner := newSSELineScanner(pr)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "[DONE]" || payload == "" {
			continue
		}
		var probe struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &probe) != nil {
			continue
		}
		for _, c := range probe.Choices {
			if c.Delta.Content == "" {
				continue
			}
			chunk := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role":  "model",
							"parts": []map[string]any{{"text": c.Delta.Content}},
						},
						"index": 0,
					},
				},
			}
			raw, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", string(raw))
			flusher.Flush()
		}
	}
}

func embeddingsV1Handler(w http.ResponseWriter, r *http.Request) {
	dispatchMedia(w, r, store.MediaCategoryEmbedding, "/embeddings")
}

func imagesV1Handler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/images")
	dispatchMedia(w, r, store.MediaCategoryTextToImage, "/images"+rest)
}

func audioV1Handler(w http.ResponseWriter, r *http.Request) {

	rest := strings.TrimPrefix(r.URL.Path, "/v1/audio")
	if strings.HasPrefix(rest, "/transcriptions") || strings.HasPrefix(rest, "/translations") {
		transcriptionsHandler(w, r)
		return
	}
	dispatchMedia(w, r, store.MediaCategoryTTS, "/audio"+rest)
}

func searchV1Handler(w http.ResponseWriter, r *http.Request) {
	dispatchMedia(w, r, store.MediaCategoryWebFetch, "/search")
}

func webV1Handler(w http.ResponseWriter, r *http.Request) {

	if strings.HasPrefix(r.URL.Path, "/v1/web/fetch") {
		webFetchHandler(w, r)
		return
	}
	dispatchMedia(w, r, store.MediaCategoryWebFetch, "/web")
}

func apiV1Handler(w http.ResponseWriter, r *http.Request) {
	suffix := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/api"), "/")
	switch {
	case suffix == "chat/completions" || suffix == "chat" || suffix == "completions":
		chatCompletionsHandler(w, r)
	case suffix == "messages":
		messagesV1Handler(w, r)
	case suffix == "responses":
		responsesV1Handler(w, r)
	case suffix == "embeddings":
		embeddingsV1Handler(w, r)
	case suffix == "models":
		modelsHandler(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":     "unknown /v1/api path: " + suffix,
			"supported": []string{"chat/completions", "messages", "responses", "embeddings", "models"},
		})
	}
}

func dispatchMedia(w http.ResponseWriter, r *http.Request, category, suffix string) {
	d, _ := store.Open()
	providers, err := store.ListMediaProviders(d, category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var picked *store.MediaProvider
	for i := range providers {
		if providers[i].IsActive {
			picked = &providers[i]
			break
		}
	}
	if picked == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"category": category,
			"message":  "no active media provider configured for this category — add one in Media Providers",
			"hint":     "POST /api/media-providers with category=" + category,
		})
		return
	}
	endpoint := strings.TrimRight(picked.BaseURL, "/") + suffix
	body, _ := io.ReadAll(io.LimitReader(r.Body, 32*1024*1024))
	defer r.Body.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	upstreamReq, err := http.NewRequestWithContext(ctx, r.Method, endpoint, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "build req: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for k, vs := range r.Header {
		if strings.EqualFold(k, "Host") || strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			upstreamReq.Header.Add(k, v)
		}
	}
	if picked.APIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+picked.APIKey)
	}
	upstreamResp, err := router.OutboundClient(ctx).Do(upstreamReq)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstreamResp.Body.Close()

	for k, vs := range upstreamResp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(upstreamResp.StatusCode)
	_, _ = io.Copy(w, upstreamResp.Body)
}

var mediaHTTPClient = &http.Client{Timeout: 60 * time.Second}
