// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/trigger-schedule.md

package triggers

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

type Event struct {
	Key     string
	Payload map[string]string
}

type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Default  string `json:"default,omitempty"`
	Help     string `json:"help,omitempty"`
	Required bool   `json:"required,omitempty"`
}

type Type interface {
	ID() string
	Name() string
	Mode() string
	ConfigSchema() []Field
	PayloadKeys() []string

	Check(config map[string]string, state string) (events []Event, newState string, err error)

	OnWebhook(config map[string]string, body []byte) ([]Event, error)
}

var (
	regMu    sync.RWMutex
	registry = map[string]Type{}
)

func Register(t Type) {
	regMu.Lock()
	registry[t.ID()] = t
	regMu.Unlock()
}

func GetType(id string) (Type, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	t, ok := registry[id]
	return t, ok
}

func ListTypes() []Type {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Type, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	return out
}

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

const maxValLen = 8000

func renderTemplate(tmpl string, payload map[string]string) string {
	return tmplRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		v := payload[tmplRe.FindStringSubmatch(m)[1]]
		if len(v) > maxValLen {
			v = v[:maxValLen] + "…"
		}
		return v
	})
}
