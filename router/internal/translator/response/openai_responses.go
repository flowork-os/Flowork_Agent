// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "openai", To: "openai-responses"}, translator.DirResponse, OpenAIToResponses)
}

func OpenAIToResponses(body map[string]any) map[string]any {
	var text string
	if ch, ok := body["choices"].([]any); ok && len(ch) > 0 {
		if c, ok := ch[0].(map[string]any); ok {
			if msg, ok := c["message"].(map[string]any); ok {
				text, _ = msg["content"].(string)
			}
		}
	}
	usageIn, _ := body["usage"].(map[string]any)
	return map[string]any{
		"id":         body["id"],
		"object":     "response",
		"created_at": body["created"],
		"model":      body["model"],
		"status":     "completed",
		"output": []map[string]any{{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "output_text", "text": text},
			},
		}},
		"usage": map[string]any{
			"input_tokens":  int64Of(usageIn["prompt_tokens"]),
			"output_tokens": int64Of(usageIn["completion_tokens"]),
			"total_tokens":  int64Of(usageIn["total_tokens"]),
		},
	}
}
