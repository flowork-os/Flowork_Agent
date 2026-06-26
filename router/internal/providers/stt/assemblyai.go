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
	"time"
)

func init() { Register(&assemblyAIProvider{}) }

type assemblyAIProvider struct{}

func (a *assemblyAIProvider) Name() string { return "assemblyai" }

func (a *assemblyAIProvider) Transcribe(ctx context.Context, req Request) (Result, error) {
	base := defaultStr(req.BaseURL, "https://api.assemblyai.com/v2")

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/upload", bytes.NewReader(req.Audio))
	if err != nil {
		return Result{}, fmt.Errorf("build upload: %w", err)
	}
	uploadReq.Header.Set("Authorization", req.APIKey)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	upRaw, err := doJSONRequest(uploadReq)
	if err != nil {
		return Result{}, fmt.Errorf("upload: %w", err)
	}
	var up struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(upRaw, &up); err != nil || up.UploadURL == "" {
		return Result{}, fmt.Errorf("upload parse: %v body=%s", err, head(upRaw))
	}

	body := map[string]any{
		"audio_url": up.UploadURL,
	}
	if req.Model != "" {
		body["speech_model"] = req.Model
	}
	if req.Language != "" {
		body["language_code"] = req.Language
	} else {
		body["language_detection"] = true
	}
	subBody, _ := json.Marshal(body)
	subReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/transcript", bytes.NewReader(subBody))
	if err != nil {
		return Result{}, fmt.Errorf("build submit: %w", err)
	}
	subReq.Header.Set("Authorization", req.APIKey)
	subReq.Header.Set("Content-Type", "application/json")
	subRaw, err := doJSONRequest(subReq)
	if err != nil {
		return Result{}, fmt.Errorf("submit: %w", err)
	}
	var submitted struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(subRaw, &submitted); err != nil || submitted.ID == "" {
		return Result{}, fmt.Errorf("submit parse: %v body=%s", err, head(subRaw))
	}

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-time.After(2 * time.Second):
		}
		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/transcript/"+submitted.ID, nil)
		if err != nil {
			return Result{}, fmt.Errorf("build poll: %w", err)
		}
		pollReq.Header.Set("Authorization", req.APIKey)
		pollRaw, err := doJSONRequest(pollReq)
		if err != nil {
			return Result{}, fmt.Errorf("poll: %w", err)
		}
		var poll struct {
			Status        string  `json:"status"`
			Text          string  `json:"text"`
			LanguageCode  string  `json:"language_code,omitempty"`
			AudioDuration float64 `json:"audio_duration,omitempty"`
			Error         string  `json:"error,omitempty"`
		}
		if err := json.Unmarshal(pollRaw, &poll); err != nil {
			return Result{}, fmt.Errorf("poll parse: %w", err)
		}
		switch poll.Status {
		case "completed":
			return Result{
				Text:         poll.Text,
				Language:     poll.LanguageCode,
				DurationSec:  poll.AudioDuration,
				ResponseJSON: pollRaw,
			}, nil
		case "error":
			return Result{}, fmt.Errorf("assemblyai: %s", poll.Error)
		}

	}
	return Result{}, fmt.Errorf("assemblyai: poll timeout after 120s")
}
