// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Translator → dok lock/gui/Translator.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/store"
	"github.com/flowork-os/flowork_Router/internal/translator/helpers"
)

func translatorRouterHandler(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/translator")
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		translatorListUpsertHandler(w, r)
		return
	}
	if strings.HasPrefix(rest, "load") {
		translatorLoadHandler(w, r)
		return
	}
	if rest == "save" {
		translatorListUpsertHandler(w, r)
		return
	}
	if rest == "translate" {
		translatorTranslateHandler(w, r)
		return
	}
	if rest == "send" {
		translatorSendHandler(w, r)
		return
	}
	if rest == "console-logs" {
		translatorConsoleLogsHandler(w, r)
		return
	}
	if rest == "console-logs/stream" {
		translatorConsoleLogsStreamHandler(w, r)
		return
	}

	translatorCRUDHandler(w, r, rest)
}

func translatorListUpsertHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListTranslatorDrafts(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var t store.TranslatorDraft
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if t.SourceFormat == "" || t.TargetFormat == "" {
			http.Error(w, "sourceFormat + targetFormat required", http.StatusBadRequest)
			return
		}
		if err := store.UpsertTranslatorDraft(d, &t); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func translatorLoadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/translator/load/")
	if id == "" || id == r.URL.Path {
		id = r.URL.Query().Get("id")
	}
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	d, _ := store.Open()
	t, err := store.GetTranslatorDraft(d, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func translatorCRUDHandler(w http.ResponseWriter, r *http.Request, id string) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		t, err := store.GetTranslatorDraft(d, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if t == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodDelete:
		if err := store.DeleteTranslatorDraft(d, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func translatorTranslateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SourceFormat string         `json:"sourceFormat"`
		TargetFormat string         `json:"targetFormat"`
		Payload      map[string]any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	out, err := translateFormat(body.SourceFormat, body.TargetFormat, body.Payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sourceFormat": body.SourceFormat,
		"targetFormat": body.TargetFormat,
		"result":       out,
	})
}

func translatorSendHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		SourceFormat string         `json:"sourceFormat"`
		TargetFormat string         `json:"targetFormat"`
		Payload      map[string]any `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.SourceFormat == "" {
		body.SourceFormat = "openai"
	}
	if body.TargetFormat == "" {
		body.TargetFormat = "openai"
	}
	canonical, err := normalizeToCanonical(body.SourceFormat, body.Payload)
	if err != nil {
		http.Error(w, "normalize: "+err.Error(), http.StatusBadRequest)
		return
	}
	if canonical.MaxTokens <= 0 {
		canonical.MaxTokens = helpers.DefaultMaxTokens
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	resp, status, derr := router.DispatchChatCompletion(ctx, *canonical)
	if derr != nil {
		writeJSON(w, status, map[string]any{"error": derr.Error(), "stage": "dispatch"})
		return
	}
	out := formatResponseAs(body.TargetFormat, resp)
	writeJSON(w, http.StatusOK, map[string]any{
		"sourceFormat": body.SourceFormat,
		"targetFormat": body.TargetFormat,
		"response":     out,
		"usage": map[string]any{
			"promptTokens":     resp.Usage.PromptTokens,
			"completionTokens": resp.Usage.CompletionTokens,
			"totalTokens":      resp.Usage.TotalTokens,
		},
	})
}

func normalizeToCanonical(format string, payload map[string]any) (*router.OpenAIRequest, error) {
	raw, _ := json.Marshal(payload)
	req := &router.OpenAIRequest{}
	switch format {
	case "openai", "":
		if err := json.Unmarshal(raw, req); err != nil {
			return nil, err
		}
	case "anthropic":
		var a struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			System    any    `json:"system"`
			Messages  []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		req.Model = a.Model
		req.MaxTokens = a.MaxTokens
		if s := anyToText(a.System); s != "" {
			req.Messages = append(req.Messages, router.OpenAIMessage{Role: "system", Content: s})
		}
		for _, m := range a.Messages {
			req.Messages = append(req.Messages, router.OpenAIMessage{Role: m.Role, Content: anyToText(m.Content)})
		}
	case "gemini":
		var g struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, err
		}
		for _, c := range g.Contents {
			role := c.Role
			if role == "model" {
				role = "assistant"
			}
			if role == "" {
				role = "user"
			}
			var txt string
			for _, p := range c.Parts {
				txt += p.Text
			}
			req.Messages = append(req.Messages, router.OpenAIMessage{Role: role, Content: txt})
		}
	default:
		return nil, fmt.Errorf("unsupported sourceFormat: %s", format)
	}
	return req, nil
}

func formatResponseAs(format string, resp *router.OpenAIResponse) any {
	var content string
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	switch format {
	case "anthropic":
		return map[string]any{
			"id": resp.ID, "type": "message", "role": "assistant", "model": resp.Model,
			"content":     []map[string]any{{"type": "text", "text": content}},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens": resp.Usage.PromptTokens, "output_tokens": resp.Usage.CompletionTokens,
			},
		}
	case "gemini":
		return map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": content}}},
				"finishReason": "STOP", "index": 0,
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount": resp.Usage.PromptTokens, "candidatesTokenCount": resp.Usage.CompletionTokens,
				"totalTokenCount": resp.Usage.TotalTokens,
			},
		}
	default:
		return resp
	}
}

func anyToText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, item := range t {
			if mm, ok := item.(map[string]any); ok {
				if txt, ok := mm["text"].(string); ok {
					parts = append(parts, txt)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func translatorConsoleLogsStreamHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	d, _ := store.Open()
	entries, _ := store.ListRecent(d, 20, "", "")
	snap, _ := json.Marshal(map[string]any{"data": entries, "count": len(entries)})
	fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", string(snap))
	flusher.Flush()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func translatorConsoleLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "count": 0, "phase": "phase2_pending"})
}

func translateFormat(src, dst string, payload map[string]any) (map[string]any, error) {
	if src == dst || src == "" || dst == "" {
		return payload, nil
	}

	canonical := payload
	switch src {
	case "anthropic":
		canonical = anthropicToOpenAI(payload)
	case "gemini":
		canonical = geminiToOpenAI(payload)
	case "openai":

	default:
		return nil, errInvalid("unknown sourceFormat: " + src)
	}

	switch dst {
	case "openai":
		return canonical, nil
	case "anthropic":
		return openAIToAnthropic(canonical), nil
	case "gemini":
		return openAIToGemini(canonical), nil
	}
	return nil, errInvalid("unknown targetFormat: " + dst)
}

func anthropicToOpenAI(p map[string]any) map[string]any {
	out := map[string]any{
		"model": p["model"],
	}
	var msgs []map[string]any
	if sys, ok := p["system"].(string); ok && sys != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": sys})
	}
	if arr, ok := p["messages"].([]any); ok {
		for _, m := range arr {
			if mm, ok := m.(map[string]any); ok {
				msgs = append(msgs, mm)
			}
		}
	}
	out["messages"] = msgs
	if mt, ok := p["max_tokens"]; ok {
		out["max_tokens"] = mt
	}
	if t, ok := p["temperature"]; ok {
		out["temperature"] = t
	}
	return out
}

func geminiToOpenAI(p map[string]any) map[string]any {
	out := map[string]any{
		"model": p["model"],
	}
	var msgs []map[string]any
	if arr, ok := p["contents"].([]any); ok {
		for _, m := range arr {
			if mm, ok := m.(map[string]any); ok {
				role := "user"
				if r, ok := mm["role"].(string); ok && r == "model" {
					role = "assistant"
				}
				var text string
				if parts, ok := mm["parts"].([]any); ok {
					for _, prt := range parts {
						if pm, ok := prt.(map[string]any); ok {
							if t, ok := pm["text"].(string); ok {
								text += t
							}
						}
					}
				}
				msgs = append(msgs, map[string]any{"role": role, "content": text})
			}
		}
	}
	out["messages"] = msgs
	return out
}

func openAIToAnthropic(p map[string]any) map[string]any {
	out := map[string]any{
		"model": p["model"],
	}
	var system string
	var msgs []map[string]any
	if arr, ok := p["messages"].([]any); ok {
		for _, m := range arr {
			if mm, ok := m.(map[string]any); ok {
				role, _ := mm["role"].(string)
				if role == "system" {
					if s, ok := mm["content"].(string); ok {
						system = s
					}
					continue
				}
				msgs = append(msgs, mm)
			}
		}
	} else if arr, ok := p["messages"].([]map[string]any); ok {
		for _, mm := range arr {
			role, _ := mm["role"].(string)
			if role == "system" {
				if s, ok := mm["content"].(string); ok {
					system = s
				}
				continue
			}
			msgs = append(msgs, mm)
		}
	}
	if system != "" {
		out["system"] = system
	}
	out["messages"] = msgs
	if mt, ok := p["max_tokens"]; ok {
		out["max_tokens"] = mt
	} else {
		out["max_tokens"] = 1024
	}
	if t, ok := p["temperature"]; ok {
		out["temperature"] = t
	}
	return out
}

func openAIToGemini(p map[string]any) map[string]any {
	out := map[string]any{"model": p["model"]}
	arr, _ := p["messages"].([]any)
	var contents []map[string]any
	for _, m := range arr {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		content, _ := mm["content"].(string)
		if role == "system" {
			if len(arr) > 1 {
				out["systemInstruction"] = map[string]any{"parts": []map[string]any{{"text": content}}}
				continue
			}
			role = "user"
		}
		if role == "assistant" {
			role = "model"
		} else {
			role = "user"
		}
		contents = append(contents, map[string]any{"role": role, "parts": []map[string]any{{"text": content}}})
	}
	out["contents"] = contents
	gc := map[string]any{}
	if t, ok := p["temperature"]; ok {
		gc["temperature"] = t
	}
	if mt, ok := p["max_tokens"]; ok {
		gc["maxOutputTokens"] = mt
	}
	if len(gc) > 0 {
		out["generationConfig"] = gc
	}
	return out
}

type errString string

func (e errString) Error() string { return string(e) }

func errInvalid(msg string) error { return errString(msg) }
