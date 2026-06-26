// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package stt

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Request struct {
	Model     string
	Audio     []byte
	AudioMIME string
	Language  string
	FileName  string
	APIKey    string
	BaseURL   string
	Extra     map[string]any
}

type Result struct {
	Text         string
	Language     string
	DurationSec  float64
	ResponseJSON []byte
}

type STTProvider interface {
	Name() string
	Transcribe(ctx context.Context, req Request) (Result, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]STTProvider{}
)

func Register(p STTProvider) {
	if p == nil || p.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[p.Name()] = p
}

func Get(name string) STTProvider {
	regMu.RLock()
	defer regMu.RUnlock()
	return registry[name]
}

func List() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

var sttHTTPClient = &http.Client{Timeout: 5 * time.Minute}

func doJSONRequest(r *http.Request) ([]byte, error) {
	resp, err := sttHTTPClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, head(body))
	}
	return body, nil
}

func defaultStr(v, def string) string {
	if v == "" {
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

func resolveAudioMIME(req Request) string {
	if len(req.AudioMIME) >= 6 && req.AudioMIME[:6] == "audio/" {
		return req.AudioMIME
	}
	if req.FileName == "" {
		return defaultStr(req.AudioMIME, "application/octet-stream")
	}

	n := len(req.FileName)
	dot := -1
	for i := n - 1; i >= 0 && i > n-7; i-- {
		if req.FileName[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return defaultStr(req.AudioMIME, "application/octet-stream")
	}
	switch req.FileName[dot+1:] {
	case "mp3":
		return "audio/mpeg"
	case "mp4", "m4a":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "webm":
		return "audio/webm"
	case "aac":
		return "audio/aac"
	case "opus":
		return "audio/opus"
	}
	return defaultStr(req.AudioMIME, "application/octet-stream")
}
