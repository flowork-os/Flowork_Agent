// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package tts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Request struct {
	Model          string
	Input          string
	Voice          string
	ResponseFormat string
	Speed          float64
	APIKey         string
	BaseURL        string
	Extra          map[string]any
}

type TTSProvider interface {
	Name() string
	Speak(ctx context.Context, req Request) (audio []byte, contentType string, err error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]TTSProvider{}
)

func Register(p TTSProvider) {
	if p == nil || p.Name() == "" {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry[p.Name()] = p
}

func Get(name string) TTSProvider {
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

var ttsHTTPClient = &http.Client{Timeout: 5 * time.Minute}

func doAudioRequest(r *http.Request) ([]byte, string, error) {
	resp, err := ttsHTTPClient.Do(r)
	if err != nil {
		return nil, "", fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("upstream %d: %s", resp.StatusCode, head(body))
	}
	return body, resp.Header.Get("Content-Type"), nil
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
