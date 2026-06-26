// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func init() { Register(&deepgramProvider{}) }

type deepgramProvider struct{}

func (d *deepgramProvider) Name() string { return "deepgram" }

func (d *deepgramProvider) Transcribe(ctx context.Context, req Request) (Result, error) {
	base := defaultStr(req.BaseURL, "https://api.deepgram.com/v1/listen")
	u, err := url.Parse(base)
	if err != nil {
		return Result{}, fmt.Errorf("base url: %w", err)
	}
	q := u.Query()
	q.Set("model", defaultStr(req.Model, "nova-2"))
	q.Set("smart_format", "true")
	q.Set("punctuate", "true")
	if req.Language != "" {
		q.Set("language", req.Language)
	} else {
		q.Set("detect_language", "true")
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(req.Audio))
	if err != nil {
		return Result{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Token "+req.APIKey)
	httpReq.Header.Set("Content-Type", resolveAudioMIME(req))

	body, err := doJSONRequest(httpReq)
	if err != nil {
		return Result{}, err
	}

	var parsed struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string  `json:"transcript"`
					Confidence float64 `json:"confidence"`
					Language   string  `json:"language,omitempty"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
		Metadata struct {
			Duration float64 `json:"duration"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Result{}, fmt.Errorf("parse deepgram json: %w", err)
	}
	text := ""
	lang := ""
	if len(parsed.Results.Channels) > 0 && len(parsed.Results.Channels[0].Alternatives) > 0 {
		text = parsed.Results.Channels[0].Alternatives[0].Transcript
		lang = parsed.Results.Channels[0].Alternatives[0].Language
	}
	return Result{
		Text:         text,
		Language:     lang,
		DurationSec:  parsed.Metadata.Duration,
		ResponseJSON: body,
	}, nil
}
