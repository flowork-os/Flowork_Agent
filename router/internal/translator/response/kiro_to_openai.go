// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "kiro", To: "openai"}, translator.DirResponse, KiroToOpenAI)
}

func KiroToOpenAI(body map[string]any) map[string]any {
	var text string
	if content, ok := body["content"].(map[string]any); ok {
		text, _ = content["text"].(string)
	}
	if text == "" {
		text, _ = body["text"].(string)
	}
	return map[string]any{
		"id":     "chatcmpl-kiro",
		"object": "chat.completion",
		"model":  body["modelId"],
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     int64(0),
			"completion_tokens": int64(0),
			"total_tokens":      int64(0),
		},
	}
}
