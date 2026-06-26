// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

type voiceLang struct {
	Code   string     `json:"code"`
	Name   string     `json:"name"`
	Voices []voiceRec `json:"voices"`
}

type voiceRec struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Lang     string `json:"lang,omitempty"`
	Category string `json:"category,omitempty"`
}

type voiceEnvelope struct {
	Languages []voiceLang           `json:"languages"`
	ByLang    map[string]*voiceLang `json:"byLang"`
}

func (e *voiceEnvelope) addVoice(code string, v voiceRec) {
	if e.ByLang == nil {
		e.ByLang = map[string]*voiceLang{}
	}
	bucket, ok := e.ByLang[code]
	if !ok {
		bucket = &voiceLang{Code: code, Name: code, Voices: []voiceRec{}}
		e.ByLang[code] = bucket
	}
	for _, existing := range bucket.Voices {
		if existing.ID == v.ID {
			return
		}
	}
	bucket.Voices = append(bucket.Voices, v)
}

func (e *voiceEnvelope) finalize() {
	keys := make([]string, 0, len(e.ByLang))
	for k := range e.ByLang {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == "Custom" {
			return false
		}
		if keys[j] == "Custom" {
			return true
		}
		return keys[i] < keys[j]
	})
	e.Languages = make([]voiceLang, 0, len(keys))
	for _, k := range keys {
		b := e.ByLang[k]
		sort.Slice(b.Voices, func(i, j int) bool { return b.Voices[i].Name < b.Voices[j].Name })
		e.Languages = append(e.Languages, *b)
	}
}

func firstActiveAPIKey(vendor string) string {
	d, err := store.Open()
	if err != nil {
		return ""
	}
	providers, err := store.ListMediaProviders(d, store.MediaCategoryTTS)
	if err != nil {
		return ""
	}
	for i := range providers {
		if providers[i].IsActive && providers[i].Provider == vendor {
			return providers[i].APIKey
		}
	}
	return ""
}

func emitVoices(w http.ResponseWriter, env *voiceEnvelope, langFilter string) {
	env.finalize()
	if langFilter != "" {
		bucket, ok := env.ByLang[langFilter]
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"voices": []voiceRec{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"voices": bucket.Voices})
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func ttsVendorGet(ctx context.Context, url, authHeader, authValue string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(authHeader, authValue)
	resp, err := httpClientVoices.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, snippet(body))
	}
	return body, nil
}

var httpClientVoices = &http.Client{Timeout: 15 * time.Second}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func deepgramVoicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apiKey := firstActiveAPIKey("deepgram")
	if apiKey == "" {
		http.Error(w, "No Deepgram connection found", http.StatusBadRequest)
		return
	}
	body, err := ttsVendorGet(r.Context(), "https://api.deepgram.com/v1/models", "Authorization", "Token "+apiKey)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Deepgram: " + err.Error()})
		return
	}
	var raw struct {
		TTS []struct {
			CanonicalName string   `json:"canonical_name"`
			Name          string   `json:"name"`
			Languages     []string `json:"languages"`
		} `json:"tts"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "parse Deepgram response: " + err.Error()})
		return
	}
	env := &voiceEnvelope{}
	for _, m := range raw.TTS {
		langs := m.Languages
		if len(langs) == 0 {
			if parts := strings.Split(m.CanonicalName, "-"); len(parts) > 0 {
				langs = []string{parts[len(parts)-1]}
			} else {
				langs = []string{"en"}
			}
		}
		for _, code := range langs {
			env.addVoice(code, voiceRec{
				ID:   m.CanonicalName,
				Name: firstStrTTS(m.Name, m.CanonicalName),
				Lang: code,
			})
		}
	}
	emitVoices(w, env, r.URL.Query().Get("lang"))
}

func elevenlabsVoicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apiKey := firstActiveAPIKey("elevenlabs")
	if apiKey == "" {
		http.Error(w, "No ElevenLabs connection found", http.StatusBadRequest)
		return
	}
	body, err := ttsVendorGet(r.Context(), "https://api.elevenlabs.io/v1/voices", "xi-api-key", apiKey)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "ElevenLabs: " + err.Error()})
		return
	}
	var raw struct {
		Voices []struct {
			VoiceID           string         `json:"voice_id"`
			Name              string         `json:"name"`
			VerifiedLanguages []string       `json:"verified_languages"`
			Labels            map[string]any `json:"labels"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "parse ElevenLabs response: " + err.Error()})
		return
	}
	env := &voiceEnvelope{}
	for _, v := range raw.Voices {
		langs := append([]string{}, v.VerifiedLanguages...)
		if s, _ := v.Labels["language"].(string); s != "" {
			langs = append(langs, s)
		}
		if len(langs) == 0 {
			langs = []string{"en"}
		}
		seen := map[string]bool{}
		for _, code := range langs {
			if seen[code] {
				continue
			}
			seen[code] = true
			env.addVoice(code, voiceRec{
				ID:   v.VoiceID,
				Name: firstStrTTS(v.Name, v.VoiceID),
				Lang: code,
			})
		}
	}
	emitVoices(w, env, r.URL.Query().Get("lang"))
}

func inworldVoicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apiKey := firstActiveAPIKey("inworld")
	if apiKey == "" {
		http.Error(w, "No Inworld connection found", http.StatusBadRequest)
		return
	}
	body, err := ttsVendorGet(r.Context(), "https://api.inworld.ai/tts/v1/voices", "Authorization", "Basic "+apiKey)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Inworld: " + err.Error()})
		return
	}
	var raw struct {
		Voices []struct {
			VoiceID   string   `json:"voiceId"`
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Languages []string `json:"languages"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "parse Inworld response: " + err.Error()})
		return
	}
	env := &voiceEnvelope{}
	for _, v := range raw.Voices {
		id := firstStrTTS(v.VoiceID, v.ID, v.Name)
		langs := v.Languages
		if len(langs) == 0 {
			langs = []string{"en"}
		}
		for _, code := range langs {
			env.addVoice(code, voiceRec{
				ID:   id,
				Name: firstStrTTS(v.Name, id),
				Lang: code,
			})
		}
	}
	emitVoices(w, env, r.URL.Query().Get("lang"))
}

var minimaxVoiceEndpoints = map[string]string{
	"minimax":    "https://api.minimax.io/v1/get_voice",
	"minimax-cn": "https://api.minimaxi.com/v1/get_voice",
}

var minimaxVoiceGroups = []struct{ key, label string }{
	{"system_voice", "System"},
	{"voice_cloning", "Cloned"},
	{"voice_generation", "Generated"},
	{"music_generation", "Music"},
}

func minimaxVoicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	vendor := "minimax"
	if q.Get("provider") == "minimax-cn" {
		vendor = "minimax-cn"
	}
	voiceType := q.Get("voice_type")
	if voiceType == "" {
		voiceType = "all"
	}
	apiKey := firstActiveAPIKey(vendor)
	if apiKey == "" {
		http.Error(w, "No "+vendor+" connection found", http.StatusBadRequest)
		return
	}
	endpoint := minimaxVoiceEndpoints[vendor]
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	reqBody, _ := json.Marshal(map[string]any{"voice_type": voiceType})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClientVoices.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "MiniMax: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	rawText, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var data map[string]any
	if len(rawText) > 0 {
		_ = json.Unmarshal(rawText, &data)
	}
	baseResp, _ := data["base_resp"].(map[string]any)
	if baseResp == nil {
		baseResp, _ = data["baseResp"].(map[string]any)
	}
	statusCode := 0
	if v, ok := baseResp["status_code"].(float64); ok {
		statusCode = int(v)
	} else if v, ok := baseResp["statusCode"].(float64); ok {
		statusCode = int(v)
	}
	statusMsg, _ := baseResp["status_msg"].(string)
	if statusMsg == "" {
		statusMsg, _ = baseResp["statusMsg"].(string)
	}
	if statusMsg == "" {
		statusMsg, _ = data["message"].(string)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": fmt.Sprintf("MiniMax API %d: %s", resp.StatusCode, firstStrTTS(statusMsg, snippet(rawText))),
		})
		return
	}
	if statusCode != 0 {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": firstStrTTS(statusMsg, "MiniMax voice API error"),
		})
		return
	}
	env := normalizeMiniMaxVoices(data)
	emitVoices(w, env, q.Get("lang"))
}

func normalizeMiniMaxVoices(data map[string]any) *voiceEnvelope {
	env := &voiceEnvelope{}
	for _, group := range minimaxVoiceGroups {
		items, _ := data[group.key].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if item == nil {
				continue
			}
			voiceID, _ := item["voice_id"].(string)
			if voiceID == "" {
				voiceID, _ = item["voiceId"].(string)
			}
			if voiceID == "" {
				continue
			}
			voiceName, _ := item["voice_name"].(string)
			if voiceName == "" {
				voiceName, _ = item["voiceName"].(string)
			}
			if voiceName == "" {
				voiceName = voiceID
			}
			lang := "Custom"
			displayName := voiceName
			if group.key == "system_voice" {
				lang = inferMiniMaxLanguage(voiceID)
			} else {
				displayName = voiceName + " · " + group.label
			}
			env.addVoice(lang, voiceRec{
				ID:       voiceID,
				Name:     displayName,
				Lang:     lang,
				Category: group.key,
			})
		}
	}
	return env
}

func inferMiniMaxLanguage(voiceID string) string {
	s := strings.TrimSpace(voiceID)
	if !strings.Contains(s, "_") {
		return "Custom"
	}
	idx := strings.Index(s, "_")
	if idx <= 0 {
		return "Custom"
	}
	return s[:idx]
}

func firstStrTTS(xs ...string) string {
	for _, s := range xs {
		if s != "" {
			return s
		}
	}
	return ""
}
