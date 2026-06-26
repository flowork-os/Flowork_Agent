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

func init() { Register(&inworldProvider{}) }

type inworldProvider struct{}

func (i *inworldProvider) Name() string { return "inworld" }

func (i *inworldProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.inworld.ai/v1/tts"
	}
	body, _ := json.Marshal(map[string]any{
		"text":     req.Input,
		"voice_id": defaultStr(req.Voice, "default"),
		"format":   defaultStr(req.ResponseFormat, "mp3"),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/synthesize", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	return doAudioRequest(r)
}
