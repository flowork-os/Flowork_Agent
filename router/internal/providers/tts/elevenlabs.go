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

func init() { Register(&elevenlabsProvider{}) }

type elevenlabsProvider struct{}

func (e *elevenlabsProvider) Name() string { return "elevenlabs" }

func (e *elevenlabsProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.elevenlabs.io/v1"
	}
	voice := defaultStr(req.Voice, "21m00Tcm4TlvDq8ikWAM")
	body, _ := json.Marshal(map[string]any{
		"text":     req.Input,
		"model_id": defaultStr(req.Model, "eleven_multilingual_v2"),
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/text-to-speech/"+voice, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "audio/mpeg")
	if req.APIKey != "" {
		r.Header.Set("xi-api-key", req.APIKey)
	}
	return doAudioRequest(r)
}
