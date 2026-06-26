// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Text To Speech → dok lock/gui/Text To Speech.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/providers/tts"
	"github.com/flowork-os/flowork_Router/internal/router"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func mediaTTSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	providers, err := store.ListMediaProviders(d, store.MediaCategoryTTS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var body struct {
		Text       string `json:"text"`
		Voice      string `json:"voice"`
		Model      string `json:"model"`
		ProviderID string `json:"providerId"`
		Format     string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	var picked *store.MediaProvider
	for i := range providers {
		if providers[i].IsActive && (body.ProviderID == "" || providers[i].ID == body.ProviderID) {
			picked = &providers[i]
			break
		}
	}
	if picked == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error":    "no active TTS provider",
			"hint":     "POST /api/media-providers with category=tts",
			"category": store.MediaCategoryTTS,
		})
		return
	}

	if strings.TrimSpace(picked.BaseURL) == "" {
		impl := tts.Get(picked.Provider)
		if impl == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":     "provider has no BaseURL to proxy and no in-process adapter",
				"provider":  picked.Provider,
				"supported": tts.List(),
			})
			return
		}
		model := body.Model
		if model == "" && len(picked.Models) > 0 {
			model = picked.Models[0]
		}
		actx, acancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer acancel()
		audio, ctype, aerr := impl.Speak(actx, tts.Request{
			Model:          model,
			Input:          body.Text,
			Voice:          body.Voice,
			ResponseFormat: body.Format,
			APIKey:         picked.APIKey,
			BaseURL:        picked.BaseURL,
		})
		if aerr != nil {
			http.Error(w, "tts adapter: "+aerr.Error(), http.StatusBadGateway)
			return
		}
		if ctype == "" {
			ctype = "audio/mpeg"
		}
		w.Header().Set("Content-Type", ctype)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(audio)
		return
	}

	upstreamBody := map[string]any{
		"model": body.Model,
		"input": body.Text,
		"voice": body.Voice,
	}
	if body.Format != "" {
		upstreamBody["response_format"] = body.Format
	}
	if upstreamBody["voice"] == "" {
		upstreamBody["voice"] = "alloy"
	}
	if upstreamBody["model"] == "" {
		if len(picked.Models) > 0 {
			upstreamBody["model"] = picked.Models[0]
		} else {
			upstreamBody["model"] = "tts-1"
		}
	}
	bodyBytes, _ := json.Marshal(upstreamBody)
	endpoint := strings.TrimRight(picked.BaseURL, "/") + "/audio/speech"
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if picked.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+picked.APIKey)
	}
	upstream, err := router.OutboundClient(ctx).Do(req)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Body.Close()
	for k, vs := range upstream.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(upstream.StatusCode)
	_, _ = io.Copy(w, upstream.Body)
}
