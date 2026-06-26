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
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

func init() { Register(&openaiProvider{}) }

type openaiProvider struct{}

func (o *openaiProvider) Name() string { return "openai" }

func (o *openaiProvider) Transcribe(ctx context.Context, req Request) (Result, error) {
	base := defaultStr(req.BaseURL, "https://api.openai.com/v1")
	endpoint := strings.TrimRight(base, "/") + "/audio/transcriptions"

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	filename := defaultStr(req.FileName, "audio")
	mime := resolveAudioMIME(req)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	hdr.Set("Content-Type", mime)
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return Result{}, fmt.Errorf("multipart part: %w", err)
	}
	if _, err := part.Write(req.Audio); err != nil {
		return Result{}, fmt.Errorf("multipart write: %w", err)
	}

	if err := mw.WriteField("model", defaultStr(req.Model, "whisper-1")); err != nil {
		return Result{}, fmt.Errorf("multipart field model: %w", err)
	}
	if req.Language != "" {
		_ = mw.WriteField("language", req.Language)
	}
	_ = mw.WriteField("response_format", "verbose_json")

	if err := mw.Close(); err != nil {
		return Result{}, fmt.Errorf("multipart close: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return Result{}, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())

	respBody, err := doJSONRequest(httpReq)
	if err != nil {
		return Result{}, err
	}

	var parsed struct {
		Text     string  `json:"text"`
		Language string  `json:"language,omitempty"`
		Duration float64 `json:"duration,omitempty"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("parse openai json: %w", err)
	}
	return Result{
		Text:         parsed.Text,
		Language:     parsed.Language,
		DurationSec:  parsed.Duration,
		ResponseJSON: respBody,
	}, nil
}
