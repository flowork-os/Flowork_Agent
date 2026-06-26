// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package routerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultChatModel = "flowork-brain"

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) ChatComplete(ctx context.Context, model, prompt string, maxTokens int) (string, error) {
	if c == nil {
		return "", fmt.Errorf("router client nil")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt kosong")
	}
	if model == "" {
		model = DefaultChatModel
	}

	body, err := json.Marshal(chatReq{
		Model:       model,
		Messages:    []chatMsg{{Role: "user", Content: prompt}},
		Stream:      false,
		MaxTokens:   maxTokens,
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	hc := &http.Client{Timeout: 0}
	resp, err := hc.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out chatResp
	if uerr := json.Unmarshal(raw, &out); uerr != nil {
		return "", fmt.Errorf("decode (status %d): %w", resp.StatusCode, uerr)
	}
	if resp.StatusCode >= 400 {
		if out.Error != nil {
			return "", fmt.Errorf("router %d: %s", resp.StatusCode, out.Error.Message)
		}
		return "", fmt.Errorf("router status %d", resp.StatusCode)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty completion (model %q configured?)", model)
	}
	return out.Choices[0].Message.Content, nil
}
