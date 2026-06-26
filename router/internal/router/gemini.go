// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type geminiPart struct {
	Text string `json:"text"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiGenConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
}
type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func buildGeminiRequest(req OpenAIRequest) geminiRequest {
	g := geminiRequest{}
	var sysParts []string
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			sysParts = append(sysParts, m.Content)
		case "assistant":
			g.Contents = append(g.Contents, geminiContent{Role: "model", Parts: []geminiPart{{Text: m.Content}}})
		default:
			g.Contents = append(g.Contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: m.Content}}})
		}
	}
	if len(sysParts) > 0 {
		g.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: strings.Join(sysParts, "\n\n")}}}
	}
	if req.Temperature != 0 || req.MaxTokens > 0 || req.TopP != 0 {
		g.GenerationConfig = &geminiGenConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
			TopP:            req.TopP,
		}
	}
	return g
}

func mapGeminiFinish(reason string) string {
	switch reason {
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST":
		return "content_filter"
	default:
		return "stop"
	}
}

func applyGeminiAuth(req *http.Request, p *store.ProviderConnection) error {
	switch p.AuthType {
	case store.AuthTypeNone:
		return nil
	case store.AuthTypeAPIKey:
		k, _ := p.Data[store.CfgAPIKey].(string)
		if k == "" {
			return fmt.Errorf("provider %s missing apiKey", p.ID)
		}
		req.Header.Set("x-goog-api-key", k)
		return nil
	default:
		return fmt.Errorf("gemini provider supports api_key or none auth (got %s)", p.AuthType)
	}
}

func geminiEndpoint(baseURL, model, action string) string {
	return strings.TrimRight(baseURL, "/") + "/models/" + url.PathEscape(model) + ":" + action
}

func forwardGemini(ctx context.Context, p *store.ProviderConnection, baseURL string, req OpenAIRequest) (*OpenAIResponse, int, error) {
	body, err := json.Marshal(buildGeminiRequest(req))
	if err != nil {
		return nil, 0, fmt.Errorf("marshal gemini: %w", err)
	}
	endpoint := geminiEndpoint(baseURL, req.Model, "generateContent")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := applyGeminiAuth(httpReq, p); err != nil {
		return nil, http.StatusUnauthorized, err
	}

	resp, err := outboundClient(ctx).Do(httpReq)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("gemini %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var gr geminiResponse
	if err := json.Unmarshal(respBody, &gr); err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("parse gemini: %w", err)
	}
	var content, finish string
	if len(gr.Candidates) > 0 {
		for _, part := range gr.Candidates[0].Content.Parts {
			content += part.Text
		}
		finish = mapGeminiFinish(gr.Candidates[0].FinishReason)
	} else {
		finish = "stop"
	}
	prompt := gr.UsageMetadata.PromptTokenCount
	completion := gr.UsageMetadata.CandidatesTokenCount
	total := gr.UsageMetadata.TotalTokenCount
	if total == 0 {
		total = prompt + completion
	}
	return &OpenAIResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []OpenAIChoice{{
			Index:        0,
			Message:      OpenAIMessage{Role: "assistant", Content: content},
			FinishReason: finish,
		}},
		Usage: OpenAIUsage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total},
	}, http.StatusOK, nil
}

func streamGemini(ctx context.Context, p *store.ProviderConnection, baseURL string, req OpenAIRequest, w http.ResponseWriter, flusher http.Flusher) (OpenAIUsage, int, error) {
	body, _ := json.Marshal(buildGeminiRequest(req))
	endpoint := geminiEndpoint(baseURL, req.Model, "streamGenerateContent") + "?alt=sse"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if err := applyGeminiAuth(httpReq, p); err != nil {
		return OpenAIUsage{}, http.StatusUnauthorized, err
	}
	resp, err := outboundClient(ctx).Do(httpReq)
	if err != nil {
		return OpenAIUsage{}, http.StatusBadGateway, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return OpenAIUsage{}, resp.StatusCode, fmt.Errorf("gemini %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{"role": "assistant"}, "")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)
	var usage OpenAIUsage
	var firstLineWritten bool
	finish := "stop"
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var gr geminiResponse
		if json.Unmarshal([]byte(payload), &gr) != nil {
			continue
		}
		if len(gr.Candidates) > 0 {
			for _, part := range gr.Candidates[0].Content.Parts {
				if part.Text != "" {
					writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{"content": part.Text}, "")
					firstLineWritten = true
				}
			}
			if gr.Candidates[0].FinishReason != "" {
				finish = mapGeminiFinish(gr.Candidates[0].FinishReason)
			}
		}
		if gr.UsageMetadata.TotalTokenCount > 0 || gr.UsageMetadata.PromptTokenCount > 0 {
			usage.PromptTokens = gr.UsageMetadata.PromptTokenCount
			usage.CompletionTokens = gr.UsageMetadata.CandidatesTokenCount
			usage.TotalTokens = gr.UsageMetadata.TotalTokenCount
		}
	}
	if err := scanner.Err(); err != nil && !firstLineWritten {
		return usage, http.StatusBadGateway, fmt.Errorf("gemini stream read: %w", err)
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	writeOpenAIDelta(w, flusher, chunkID, created, req.Model, map[string]any{}, finish)
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
	return usage, http.StatusOK, nil
}
