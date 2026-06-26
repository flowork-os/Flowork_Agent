// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func init() { Register(&openrouterProvider{}) }

type openrouterProvider struct{}

func (o *openrouterProvider) Name() string { return "openrouter" }

func (o *openrouterProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	body, _ := json.Marshal(map[string]any{
		"model":           defaultStr(req.Model, "openai/tts-1"),
		"input":           req.Input,
		"voice":           defaultStr(req.Voice, "alloy"),
		"response_format": defaultStr(req.ResponseFormat, "mp3"),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("HTTP-Referer", "https://github.com/flowork-os/Flowork-OS")
	r.Header.Set("X-Title", "flow_router")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doAudioRequest(r)
}
