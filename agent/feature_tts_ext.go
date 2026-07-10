// feature_tts_ext.go — SUARA Mr.Flow via provider TTS ROUTER (owner 2026-07-10). NON-FROZEN sibling.
//
// POST /api/tts {text, lang} → server-side POST ${FLOW_ROUTER_BASE}/v1/audio/speech
// (OpenAI-style {model,input,voice}; /v1/* exempt session router). Router milih provider TTS
// yang DIPASANG owner di GUI Media Providers (GUI = kebenaran): edgeTts/elevenlabs/openai/dll.
// Voice default COWOK: env FLOWORK_TTS_VOICE / FLOWORK_TTS_VOICE_EN → id-ID-ArdiNeural /
// en-US-GuyNeural. Belum ada provider → 502 JSON → HUD fallback ke suara browser.
// Session-gated (tidak public). Hapus file ini → fitur mati, koloni utuh.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func ttsVoiceFor(lang string) string {
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		if v := os.Getenv("FLOWORK_TTS_VOICE_EN"); v != "" {
			return v
		}
		return "en-US-GuyNeural" // male
	}
	if v := os.Getenv("FLOWORK_TTS_VOICE"); v != "" {
		return v
	}
	return "id-ID-ArdiNeural" // male, natural (Edge neural)
}

func ttsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Text string `json:"text"`
		Lang string `json:"lang"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		http.Error(w, `{"error":"text required"}`, http.StatusBadRequest)
		return
	}
	if len(body.Text) > 4000 {
		body.Text = body.Text[:4000]
	}
	payload, _ := json.Marshal(map[string]any{
		"model": "tts-1",
		"input": body.Text,
		"voice": ttsVoiceFor(body.Lang),
	})
	cl := &http.Client{Timeout: 60 * time.Second}
	resp, err := cl.Post(routerBase()+"/v1/audio/speech", "application/json", bytes.NewReader(payload))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "router TTS: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode != 200 || strings.Contains(ct, "json") {
		// belum ada provider TTS di GUI router / provider error → teruskan sbg 502 JSON (HUD fallback)
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "TTS provider belum siap", "detail": string(raw)})
		return
	}
	if ct == "" {
		ct = "audio/mpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, resp.Body)
}

func init() {
	RegisterFeature(Feature{Name: "tts", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/tts", ttsHandler)
	}})
}
