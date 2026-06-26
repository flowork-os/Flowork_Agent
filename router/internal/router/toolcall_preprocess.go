// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"encoding/json"

	"github.com/flowork-os/flowork_Router/internal/translator/helpers"
)

func preprocessToolCalls(req *OpenAIRequest) {
	if !requestLooksToolful(req) {
		return
	}

	body := map[string]any{}
	raw, err := json.Marshal(req)
	if err != nil {
		return
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return
	}
	helpers.EnsureToolCallIDs(body)
	helpers.FixMissingToolResponses(body)
	normalizeThinkingConfig(body)

	patched, err := json.Marshal(body)
	if err != nil {
		return
	}
	_ = json.Unmarshal(patched, req)
}

func requestLooksToolful(req *OpenAIRequest) bool {
	if len(req.Tools) > 2 {
		return true
	}
	for _, m := range req.Messages {
		if len(m.ToolCalls) > 2 || m.ToolCallID != "" {
			return true
		}

		c := m.Content
		for i := 0; i < len(c); i++ {
			if c[i] == ' ' || c[i] == '\t' || c[i] == '\n' {
				continue
			}
			if c[i] == '[' || c[i] == '{' {
				return true
			}
			break
		}
	}
	return false
}
