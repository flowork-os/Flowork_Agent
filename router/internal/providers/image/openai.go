// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package image

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func init() { Register(&openaiProvider{}) }

type openaiProvider struct{}

func (o *openaiProvider) Name() string { return "openai" }

func (o *openaiProvider) Generate(ctx context.Context, req Request) (*Result, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	body, _ := json.Marshal(map[string]any{
		"model":   defaultStr(req.Model, "dall-e-3"),
		"prompt":  req.Prompt,
		"n":       defaultInt(req.N, 1),
		"size":    defaultStr(req.Size, "1024x1024"),
		"quality": defaultStr(req.Quality, "standard"),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doImageRequest(r)
}

var imageHTTPClient = &http.Client{Timeout: 5 * time.Minute}

func doImageRequest(r *http.Request) (*Result, error) {
	resp, err := imageHTTPClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, head(data))
	}
	var out Result
	if err := json.Unmarshal(data, &out); err == nil && len(out.Data) > 0 {
		return &out, nil
	}

	var single struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	}
	if err := json.Unmarshal(data, &single); err == nil && (single.URL != "" || single.B64JSON != "") {
		return &Result{Data: []ResultImage{{URL: single.URL, B64JSON: single.B64JSON}}}, nil
	}
	return nil, errors.New("upstream returned no image data")
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func defaultInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func head(b []byte) string {
	if len(b) > 240 {
		return string(b[:240]) + "…"
	}
	return string(b)
}
