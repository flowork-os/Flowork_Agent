// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "gemini", To: "openai"}, translator.DirResponse, GeminiToOpenAI)
}

func GeminiToOpenAI(body map[string]any) map[string]any {
	var text string
	var finishReason string
	if cands, ok := body["candidates"].([]any); ok && len(cands) > 0 {
		if c, ok := cands[0].(map[string]any); ok {
			if cc, ok := c["content"].(map[string]any); ok {
				if parts, ok := cc["parts"].([]any); ok {
					for _, p := range parts {
						if m, ok := p.(map[string]any); ok {
							if t, _ := m["text"].(string); t != "" {
								text += t
							}
						}
					}
				}
			}
			finishReason = mapGeminiFinish(stringOf(c["finishReason"]))
		}
	}
	usage := map[string]any{"prompt_tokens": int64(0), "completion_tokens": int64(0), "total_tokens": int64(0)}
	if um, ok := body["usageMetadata"].(map[string]any); ok {
		usage["prompt_tokens"] = int64Of(um["promptTokenCount"])
		usage["completion_tokens"] = int64Of(um["candidatesTokenCount"])
		usage["total_tokens"] = int64Of(um["totalTokenCount"])
	}
	model, _ := body["modelVersion"].(string)
	return map[string]any{
		"id":     "chatcmpl-gemini",
		"object": "chat.completion",
		"model":  model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": finishReason,
		}},
		"usage": usage,
	}
}

func mapGeminiFinish(s string) string {
	switch s {
	case "STOP", "":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	}
	return "stop"
}

func stringOf(v any) string { s, _ := v.(string); return s }
