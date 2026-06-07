// Package triggers — ROADMAP 3 engine (papan kosong event-driven). Engine GENERIK; logika
// tipe (time/webhook/file-watch/…) ada di file type_*.go yang self-register via init().
// Tambah tipe baru = tambah 1 file type_*.go, TANPA menyentuh engine (plug-and-play di tingkat
// sumber; upgrade ke .fwpack wasm = increment berikut, interface tetap).
package triggers

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// Event — satu kejadian yang memicu aksi.
type Event struct {
	Key     string            // id dedup
	Payload map[string]string // {{key}} ke prompt
}

// Field — satu field config tipe (untuk GUI form).
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"` // text|path|number|secret|select
	Default  string `json:"default,omitempty"`
	Help     string `json:"help,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// Type — kontrak satu tipe trigger. Engine tak tahu logikanya.
type Type interface {
	ID() string
	Name() string
	Mode() string // "poll" | "webhook"
	ConfigSchema() []Field
	PayloadKeys() []string
	// Check (mode poll): config + state opaque → events baru + state baru.
	Check(config map[string]string, state string) (events []Event, newState string, err error)
	// OnWebhook (mode webhook): config + body → events.
	OnWebhook(config map[string]string, body []byte) ([]Event, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]Type{}
)

// Register — dipanggil dari init() tiap file type_*.go.
func Register(t Type) {
	regMu.Lock()
	registry[t.ID()] = t
	regMu.Unlock()
}

// GetType — ambil tipe by id.
func GetType(id string) (Type, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	t, ok := registry[id]
	return t, ok
}

// ListTypes — semua tipe terdaftar (untuk GUI form).
func ListTypes() []Type {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Type, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	return out
}

// parseConfig — JSON config → map string.
func parseConfig(cfg string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(cfg) == "" {
		return out
	}
	var raw map[string]any
	if json.Unmarshal([]byte(cfg), &raw) == nil {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				out[k] = s
			} else {
				b, _ := json.Marshal(v)
				out[k] = strings.Trim(string(b), `"`)
			}
		}
	}
	return out
}

var tmplRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

// maxValLen — batas panjang SATU nilai payload yang disuntik ke prompt. Payload
// webhook bersumber EKSTERNAL (≤1MB) → tanpa cap, body gede membanjiri prompt LLM
// (bakar token) + perbesar permukaan prompt-injection. 8KB cukup utk konteks nyata.
const maxValLen = 8000

// renderTemplate — ganti {{key}} dgn payload[key] (Variable ala GTM). Key tak ada → kosong.
// Nilai dipotong di maxValLen (anti banjir token dari payload eksternal).
func renderTemplate(tmpl string, payload map[string]string) string {
	return tmplRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		v := payload[tmplRe.FindStringSubmatch(m)[1]]
		if len(v) > maxValLen {
			v = v[:maxValLen] + "…"
		}
		return v
	})
}
