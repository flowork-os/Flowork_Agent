// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "commandcode", To: "openai"}, translator.DirResponse, CommandCodeToOpenAI)
}

func CommandCodeToOpenAI(body map[string]any) map[string]any {
	text, _ := body["text"].(string)
	finishReason, _ := body["finishReason"].(string)
	if finishReason == "" {
		finishReason = "stop"
	}
	usage, _ := body["usage"].(map[string]any)
	return map[string]any{
		"id":     "chatcmpl-commandcode",
		"object": "chat.completion",
		"model":  body["model"],
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": finishReason,
		}},
		"usage": map[string]any{
			"prompt_tokens":     int64Of(usage["promptTokens"]),
			"completion_tokens": int64Of(usage["completionTokens"]),
			"total_tokens":      int64Of(usage["totalTokens"]),
		},
	}
}
