// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "openai", To: "claude"}, translator.DirResponse, OpenAIToClaude)
}

func OpenAIToClaude(body map[string]any) map[string]any {
	id, _ := body["id"].(string)
	model, _ := body["model"].(string)
	var text, finishReason string
	if ch, ok := body["choices"].([]any); ok && len(ch) > 0 {
		if c, ok := ch[0].(map[string]any); ok {
			if msg, ok := c["message"].(map[string]any); ok {
				text, _ = msg["content"].(string)
			}
			finishReason, _ = c["finish_reason"].(string)
		}
	}
	stop := "end_turn"
	switch finishReason {
	case "length":
		stop = "max_tokens"
	case "tool_calls":
		stop = "tool_use"
	}
	usageIn, _ := body["usage"].(map[string]any)
	return map[string]any{
		"id":          id,
		"type":        "message",
		"role":        "assistant",
		"model":       model,
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": stop,
		"usage": map[string]any{
			"input_tokens":  int64Of(usageIn["prompt_tokens"]),
			"output_tokens": int64Of(usageIn["completion_tokens"]),
		},
	}
}
