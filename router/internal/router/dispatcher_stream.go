// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Router dispatcher.

// Streaming Dispatch (SSE).

package router

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/executors"
	"github.com/flowork-os/flowork_Router/internal/safego"
	"github.com/flowork-os/flowork_Router/internal/store"
)

// DispatchChatCompletionStream — streaming variant. Writes SSE directly to w.
// Returns (firstByteLatencyMs, totalLatencyMs, usage, status, error). Once
// the stream has begun, errors only logged; no retry possible (response
// already committed).
func DispatchChatCompletionStream(ctx context.Context, req OpenAIRequest, w http.ResponseWriter) (status int, usage OpenAIUsage, err error) {
	d, err := store.Open()
	if err != nil {
		return http.StatusInternalServerError, usage, fmt.Errorf("store open: %w", err)
	}

	settings, _ := store.LoadSettings(d)
	// No model pinned → highest-priority ON provider (priority order), same as the
	// non-stream dispatcher. DefaultModel only if no active provider has one.
	if req.Model == "" {
		if top := globalFallbackModels(d, nil); len(top) > 0 {
			req.Model = top[0]
		} else if settings != nil {
			req.Model = settings.DefaultModel
		}
	}
	if settings != nil && settings.RtkTokenSaver {
		if msgs, saved := compressMessagesRTK(req.Messages); saved > 0 {
			req.Messages = msgs
			log.Printf("flow_router RTK token saver (stream): trimmed %d chars", saved)
		}
	}

	// Brain enrichment (see dispatcher.go): inject knowledge + skills when the
	// request targets the brain model. No-op unless enabled.
	maybeEnrichBrain(ctx, &req, settings)
	// Antibody injection (mistakeenrich.go): karma-ranked mistakes → anti-halu. Fails open.
	maybeInjectAntibodies(ctx, &req, settings)
	// Instinct injection (instinctenrich.go): insting WHEN→THEN relevan di-PAKSA masuk
	// → agent SADAR kapan pakai tool/fitur. Fails open. (stream path: mirror non-stream.)
	maybeInjectInstinct(ctx, &req, settings)

	// Model manager: resolve alias / custom (→ effective model + provider pin).
	resolvedModel, pinnedProvider := resolveModel(d, req.Model)
	req.Model = resolvedModel

	// Combo resolution
	if pinnedProvider == "" {
		if combo, _ := store.GetComboByName(d, req.Model); combo != nil && len(combo.Models) > 0 {
			picked := pickComboModel(combo)
			log.Printf("flow_router combo %q (%s) stream → model %q", combo.Name, combo.Strategy, picked)
			req.Model = picked
		}
	}

	matches, err := store.FindActiveByModel(d, req.Model)
	if err != nil {
		return http.StatusInternalServerError, usage, fmt.Errorf("find provider: %w", err)
	}
	if pinnedProvider != "" {
		matches = pinProvider(d, matches, pinnedProvider)
	}
	// STABLE — DO NOT MODIFY without owner approval (locked 2026-06-15). Mirror of
	// the failover-by-priority in dispatcher.go — see the locked block there for the
	// full 2-brain / diversity / sovereign-failover rationale + the llama-server
	// context lesson. Keep both paths in sync.
	//
	// Owner doctrine: if the requested model's provider is OFF (no active match),
	// slide to the next ON provider by priority instead of failing with a 404.
	// This is pre-stream (nothing written yet), so we just need to land on a
	// servable model before the first byte. Mirrors the non-stream dispatcher.
	if len(matches) == 0 && (settings == nil || settings.FallbackStrategy != "none") {
		for _, fb := range globalFallbackModels(d, []string{req.Model}) {
			fbModel, fbPin := resolveModel(d, fb)
			m, ferr := store.FindActiveByModel(d, fbModel)
			if ferr != nil || len(m) == 0 {
				continue
			}
			if fbPin != "" {
				if m = pinProvider(d, m, fbPin); len(m) == 0 {
					continue
				}
			}
			log.Printf("flow_router priority fallback (stream): %q unavailable → trying %q (next ON provider)", req.Model, fbModel)
			// Land doctrine on the local fallback brain model (the stream path
			// otherwise never injects the constitution).
			if !isCrewLightModel(fbModel) {
				maybeInjectConstitution(ctx, &req, settings)
				maybeEnrichBrain(ctx, &req, settings)
			}
			req.Model, pinnedProvider, matches = fbModel, fbPin, m
			break
		}
	}
	if len(matches) == 0 {
		return http.StatusNotFound, usage, fmt.Errorf("no active provider supports model %q", req.Model)
	}
	if matches = filterDisabled(d, matches, req.Model); len(matches) == 0 {
		return http.StatusForbidden, usage, fmt.Errorf("model %q is disabled", req.Model)
	}

	// Inbound API-key scope: drop providers the key is not allowed to use.
	keyID := apiKeyID(ctx)
	if key := APIKeyFromContext(ctx); key != nil {
		matches = filterByAllowedProviders(matches, key)
		if len(matches) == 0 {
			return http.StatusForbidden, usage, fmt.Errorf("api key %q not permitted for any provider serving model %q", key.Name, req.Model)
		}
	}

	// Per-intent multiplexing: private prompt → local-tagged provider only.
	if settings != nil && settings.IntentRouting.Enabled && promptIsPrivate(req, settings.IntentRouting.PrivatePatterns) {
		tag := settings.IntentRouting.PrivateTag
		if tag == "" {
			tag = "local"
		}
		local := filterByTag(matches, tag)
		if len(local) == 0 {
			return http.StatusForbidden, usage, fmt.Errorf("private prompt: no provider tagged %q available — refusing to route to cloud", tag)
		}
		matches = local
	}

	// Cost-tier routing: classify request → filter by tier:* tag. Same gate
	// as the non-streaming dispatcher so behavior is consistent.
	if settings != nil && settings.CostRouting.Enabled {
		if !(settings.CostRouting.HonorExplicitModel && hasActiveProviderForModel(matches, req.Model)) {
			tier := ClassifyCost(req, settings.CostRouting)
			if tiered := filterByTier(matches, tier); len(tiered) > 0 {
				matches = tiered
			}
		}
	}

	// Fallback strategy: reorder candidates (priority_ordered = unchanged).
	if settings != nil {
		matches = applyFallbackStrategy(matches, settings.FallbackStrategy, req.Model)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return http.StatusInternalServerError, usage, fmt.Errorf("ResponseWriter does not support flushing — streaming impossible")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Try providers in priority order
	var lastErr error
	for _, p := range matches {
		req.Stream = true
		t0 := time.Now()
		u, st, err := streamFromProvider(ctx, &p, req, w, flusher)
		latencyMs := time.Since(t0).Milliseconds()
		usage = u

		// Log usage best-effort (panic-recovered so a logging bug can't crash
		// the streaming dispatcher).
		safego.GoLabel("logUsageStream", func() {
			logUsageStream(keyID, p.ID, req.Model, &u, st, err, latencyMs)
		})

		if err == nil {
			return st, u, nil
		}
		// If we already wrote any SSE, can't retry — break.
		if st == streamingPartialWrite {
			log.Printf("flow_router stream FAILED mid-stream provider=%s: %v", p.Name, err)
			return http.StatusOK, u, nil // already wrote OK header + partial
		}
		lastErr = err
		log.Printf("flow_router stream fallback model=%s provider=%s: %v", req.Model, p.Name, err)
	}
	// Nothing succeeded
	return http.StatusBadGateway, usage, fmt.Errorf("all providers failed; last: %w", lastErr)
}

// sentinel status meaning "headers/body already started, no retry possible"
const streamingPartialWrite = -1

func streamFromProvider(ctx context.Context, p *store.ProviderConnection, req OpenAIRequest, w http.ResponseWriter, flusher http.Flusher) (OpenAIUsage, int, error) {
	format, _ := p.Data[store.CfgFormat].(string)
	baseURL, _ := p.Data[store.CfgBaseURL].(string)
	if baseURL == "" {
		return OpenAIUsage{}, 0, fmt.Errorf("provider %s missing baseUrl", p.ID)
	}
	// Power saver (idle-sleep): wake the local engine if it was unloaded, and hold it
	// loaded until this stream returns. No-op for cloud. See router/llm_idle_sleep.go.
	defer wakeLocalIfNeeded(baseURL)()
	// Vendor executor registry: when a pluggable executor is registered for
	// this format, delegate. Otherwise fall through to the built-in handlers.
	if ex := executors.Get(format); ex != nil {
		u, st, err := ex.Stream(ctx, p, executorRequest(req), w, flusher)
		return OpenAIUsage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		}, st, err
	}
	switch format {
	case "anthropic":
		if hasToolContext(req) {
			return streamAnthropicWithTools(ctx, p, baseURL, req, w, flusher)
		}
		return streamAnthropic(ctx, p, baseURL, req, w, flusher)
	case "openai", "":
		return streamOpenAICompat(ctx, p, baseURL, req, w, flusher)
	case "gemini":
		return streamGemini(ctx, p, baseURL, req, w, flusher)
	default:
		return OpenAIUsage{}, 0, fmt.Errorf("streaming for format %q not yet implemented", format)
	}
}

// executorRequest converts the internal OpenAIRequest into the slim Request
// used by the executor framework, so /internal/executors/ stays cycle-free.
func executorRequest(r OpenAIRequest) executors.Request {
	msgs := make([]executors.Message, len(r.Messages))
	for i, m := range r.Messages {
		msgs[i] = executors.Message{Role: m.Role, Content: m.Content}
	}
	return executors.Request{
		Model:       r.Model,
		Messages:    msgs,
		MaxTokens:   r.MaxTokens,
		Temperature: r.Temperature,
		TopP:        r.TopP,
		Stream:      r.Stream,
	}
}

// streamOpenAICompat — issue stream:true to upstream, pipe SSE 1:1.
// Track usage from final non-data line / usage chunk if upstream provides it.
func streamOpenAICompat(ctx context.Context, p *store.ProviderConnection, baseURL string, req OpenAIRequest, w http.ResponseWriter, flusher http.Flusher) (OpenAIUsage, int, error) {
	req.Stream = true
	body, _ := json.Marshal(req)
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if err := applyAuth(httpReq, p); err != nil {
		return OpenAIUsage{}, http.StatusUnauthorized, err
	}
	resp, err := outboundClient(ctx).Do(httpReq)
	if err != nil {
		return OpenAIUsage{}, http.StatusBadGateway, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return OpenAIUsage{}, resp.StatusCode, fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	// Flush 200 OK header to client immediately
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	var usage OpenAIUsage
	var firstLineWritten bool
	for scanner.Scan() {
		line := scanner.Bytes()
		// Pass through as-is, including blank line separators
		_, werr := w.Write(line)
		if werr != nil {
			return usage, streamingPartialWrite, werr
		}
		_, _ = w.Write([]byte("\n"))
		firstLineWritten = true

		// Inspect data: lines for usage stats
		if bytes.HasPrefix(line, []byte("data: ")) {
			payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))
			if !bytes.Equal(payload, []byte("[DONE]")) && len(payload) > 0 {
				var probe struct {
					Usage   *OpenAIUsage `json:"usage,omitempty"`
					Choices []struct {
						FinishReason string `json:"finish_reason,omitempty"`
					} `json:"choices"`
				}
				if jsonErr := json.Unmarshal(payload, &probe); jsonErr == nil {
					if probe.Usage != nil {
						usage = *probe.Usage
					}
				}
			}
		}
		flusher.Flush()
	}
	if err := scanner.Err(); err != nil {
		if firstLineWritten {
			return usage, streamingPartialWrite, err
		}
		return usage, http.StatusBadGateway, err
	}
	return usage, http.StatusOK, nil
}

// Anthropic streaming events we care about:
//
//	message_start         → emit role:"assistant" start chunk
//	content_block_delta   → emit OpenAI delta.content
//	message_delta         → carries stop_reason + usage.output_tokens
//	message_stop          → emit final chunk with finish_reason + [DONE]
//	ping                  → ignore (keepalive)
//	error                 → propagate as SSE error event
//
// Ref: https://docs.anthropic.com/en/api/messages-streaming
func streamAnthropic(ctx context.Context, p *store.ProviderConnection, baseURL string, req OpenAIRequest, w http.ResponseWriter, flusher http.Flusher) (OpenAIUsage, int, error) {
	anthrReq := AnthropicRequest{
		Model:       normalizeClaudeModel(req.Model),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      true,
	}
	if anthrReq.MaxTokens <= 0 {
		anthrReq.MaxTokens = 4096
	}
	var sysParts []string
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			sysParts = append(sysParts, m.Content)
		case "user", "assistant":
			anthrReq.Messages = append(anthrReq.Messages, AnthropicMessage{Role: m.Role, Content: m.Content})
		}
	}
	if len(sysParts) > 0 {
		anthrReq.System = strings.Join(sysParts, "\n\n")
	}

	body, _ := json.Marshal(anthrReq)
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "claude-cli/1.0.0 (flow_router)")
	if err := applyAuth(httpReq, p); err != nil {
		return OpenAIUsage{}, http.StatusUnauthorized, err
	}
	resp, err := outboundClient(ctx).Do(httpReq)
	if err != nil {
		return OpenAIUsage{}, http.StatusBadGateway, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return OpenAIUsage{}, resp.StatusCode, fmt.Errorf("anthropic %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// Emit initial role:"assistant" delta chunk
	writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{"role": "assistant"}, "")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	var usage OpenAIUsage
	var firstLineWritten bool
	var stopReason string
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Message struct {
				ID    string `json:"id"`
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
		}
		if jerr := json.Unmarshal([]byte(payload), &ev); jerr != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			usage.PromptTokens = ev.Message.Usage.InputTokens
		case "content_block_start":
			if ev.ContentBlock.Type == "text" && ev.ContentBlock.Text != "" {
				writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{"content": ev.ContentBlock.Text}, "")
				firstLineWritten = true
			}
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{"content": ev.Delta.Text}, "")
				firstLineWritten = true
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				usage.CompletionTokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			fr := "stop"
			switch stopReason {
			case "end_turn", "stop_sequence":
				fr = "stop"
			case "max_tokens":
				fr = "length"
			case "tool_use":
				fr = "tool_calls"
			}
			writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{}, fr)
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
			// Final [DONE] marker
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			return usage, http.StatusOK, nil
		case "error":
			// Anthropic streaming error event — propagate
			_, _ = fmt.Fprintf(w, "data: {\"error\":{\"type\":\"upstream\",\"message\":%q}}\n\n", payload)
			flusher.Flush()
			return usage, streamingPartialWrite, fmt.Errorf("anthropic stream error: %s", payload)
		}
	}
	if err := scanner.Err(); err != nil {
		if firstLineWritten {
			return usage, streamingPartialWrite, err
		}
		return usage, http.StatusBadGateway, err
	}
	// Stream ended without explicit message_stop — best effort terminator.
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
	return usage, http.StatusOK, nil
}

// writeOpenAIDelta — emit one OpenAI-format SSE chunk.
func writeOpenAIDelta(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model string, delta map[string]any, finishReason string) {
	chunk := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": delta,
				"finish_reason": func() any {
					if finishReason == "" {
						return nil
					}
					return finishReason
				}(),
			},
		},
	}
	raw, _ := json.Marshal(chunk)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()
}

// logUsageStream — best-effort usage log for streaming dispatch.
func logUsageStream(apiKeyID, providerID, model string, usage *OpenAIUsage, status int, errIn error, latencyMs int64) {
	d, err := store.Open()
	if err != nil {
		return
	}
	entry := &store.LogEntry{
		APIKeyID:   apiKeyID,
		ProviderID: providerID,
		Model:      model,
		StatusCode: status,
		LatencyMs:  latencyMs,
	}
	if errIn != nil {
		entry.Error = errIn.Error()
	}
	if usage != nil {
		entry.PromptTokens = usage.PromptTokens
		entry.CompletionTokens = usage.CompletionTokens
		entry.TotalTokens = usage.TotalTokens
		entry.CostUsd = estimateCost(model, usage.PromptTokens, usage.CompletionTokens)
	}
	_ = store.LogRequest(d, entry)
}
