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

func init() { Register(&googleTtsProvider{}) }

type googleTtsProvider struct{}

func (g *googleTtsProvider) Name() string { return "googleTts" }

func (g *googleTtsProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {
	base := req.BaseURL
	if base == "" {
		base = "https://texttospeech.googleapis.com/v1"
	}
	body, _ := json.Marshal(map[string]any{
		"input": map[string]any{"text": req.Input},
		"voice": map[string]any{
			"languageCode": "en-US",
			"name":         defaultStr(req.Voice, "en-US-Studio-O"),
		},
		"audioConfig": map[string]any{
			"audioEncoding": "MP3",
			"speakingRate":  req.Speed,
		},
	})
	url := base + "/text:synthesize?key=" + req.APIKey
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	r.Header.Set("Content-Type", "application/json")
	return doAudioRequest(r)
}
