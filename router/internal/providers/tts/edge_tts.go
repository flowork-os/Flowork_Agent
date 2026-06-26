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

func init() { Register(&edgeTtsProvider{}) }

type edgeTtsProvider struct{}

func (e *edgeTtsProvider) Name() string { return "edgeTts" }

func (e *edgeTtsProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {
	base := req.BaseURL
	if base == "" {
		base = "http://127.0.0.1:5050"
	}
	body, _ := json.Marshal(map[string]any{
		"text":  req.Input,
		"voice": defaultStr(req.Voice, "en-US-AriaNeural"),
		"rate":  "+0%",
	})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/tts", bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	r.Header.Set("Content-Type", "application/json")
	return doAudioRequest(r)
}
