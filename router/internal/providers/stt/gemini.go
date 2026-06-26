// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package stt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func init() { Register(&geminiProvider{}) }

type geminiProvider struct{}

func (g *geminiProvider) Name() string { return "gemini" }

func (g *geminiProvider) Transcribe(ctx context.Context, req Request) (Result, error) {
	base := defaultStr(req.BaseURL, "https://generativelanguage.googleapis.com/v1beta")
	model := defaultStr(req.Model, "gemini-1.5-flash")

	mime := resolveAudioMIME(req)
	if mime == "application/octet-stream" {

		mime = "audio/mpeg"
	}

	prompt := "Transcribe the spoken content of this audio verbatim. Return only the transcript, no commentary."
	if req.Language != "" {
		prompt += " The expected language is " + req.Language + "."
	}

	body := map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{
				{"text": prompt},
				{"inline_data": map[string]any{
					"mime_type": mime,
					"data":      base64.StdEncoding.EncodeToString(req.Audio),
				}},
			},
		}},
	}
	raw, _ := json.Marshal(body)

	u, err := url.Parse(fmt.Sprintf("%s/models/%s:generateContent", base, url.PathEscape(model)))
	if err != nil {
		return Result{}, fmt.Errorf("url: %w", err)
	}
	q := u.Query()
	q.Set("key", req.APIKey)
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(raw))
	if err != nil {
		return Result{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	respBody, err := doJSONRequest(httpReq)
	if err != nil {
		return Result{}, err
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("parse gemini json: %w", err)
	}
	text := ""
	if len(parsed.Candidates) > 0 {
		for _, p := range parsed.Candidates[0].Content.Parts {
			text += p.Text
		}
	}
	return Result{
		Text:         text,
		Language:     req.Language,
		ResponseJSON: respBody,
	}, nil
}
