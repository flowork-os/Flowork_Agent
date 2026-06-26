// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"encoding/json"
	"strings"
)

type StripList []string

func stripContentTypes(req *OpenAIRequest, list StripList) {
	if len(list) == 0 {
		return
	}
	strip := map[string]bool{}
	for _, c := range list {
		strip[strings.ToLower(strings.TrimSpace(c))] = true
	}
	for i, m := range req.Messages {

		raw := strings.TrimSpace(m.Content)
		if !strings.HasPrefix(raw, "[") {
			continue
		}
		var parts []map[string]any
		if err := json.Unmarshal([]byte(raw), &parts); err != nil {
			continue
		}
		kept := parts[:0]
		for _, p := range parts {
			typ, _ := p["type"].(string)
			t := strings.ToLower(typ)
			drop := false
			switch t {
			case "image_url", "image":
				drop = strip["image"]
			case "audio_url", "input_audio":
				drop = strip["audio"]
			}
			if !drop {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			req.Messages[i].Content = ""
			continue
		}
		raw2, err := json.Marshal(kept)
		if err != nil {
			continue
		}
		req.Messages[i].Content = string(raw2)
	}
}

func normalizeThinkingConfig(body map[string]any) {
	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return
	}
	last, _ := msgs[len(msgs)-1].(map[string]any)
	role, _ := last["role"].(string)
	if role == "user" {
		return
	}

	for _, key := range []string{"thinking", "reasoning", "enable_thinking"} {
		delete(body, key)
	}
}
