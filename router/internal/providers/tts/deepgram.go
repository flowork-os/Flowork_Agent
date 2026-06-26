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

func init() { Register(&deepgramProvider{}) }

type deepgramProvider struct{}

func (d *deepgramProvider) Name() string { return "deepgram" }

func (d *deepgramProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://api.deepgram.com/v1"
	}
	model := defaultStr(req.Model, "aura-asteria-en")
	if req.Voice != "" {
		model = req.Voice
	}
	url := base + "/speak?model=" + model + "&encoding=mp3"
	body, _ := json.Marshal(map[string]any{"text": req.Input})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	r.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		r.Header.Set("Authorization", "Token "+req.APIKey)
	}
	return doAudioRequest(r)
}
