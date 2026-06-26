// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package response

import (
	"github.com/flowork-os/flowork_Router/internal/translator"
)

func init() {
	translator.Register(translator.Pair{From: "claude", To: "openai"}, translator.DirResponse, ClaudeToOpenAI)
}

func ClaudeToOpenAI(body map[string]any) map[string]any {
	id, _ := body["id"].(string)
	model, _ := body["model"].(string)
	stop, _ := body["stop_reason"].(string)
	finishReason := mapAnthropicStop(stop)

	var text string
	if blocks, ok := body["content"].([]any); ok {
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok {
				if m["type"] == "text" {
					if t, _ := m["text"].(string); t != "" {
						text += t
					}
				}
			}
		}
	}

	usageIn, _ := body["usage"].(map[string]any)
	usage := buildOpenAIUsageFromAnthropic(usageIn)

	return map[string]any{
		"id":     id,
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

func buildOpenAIUsageFromAnthropic(usageIn map[string]any) map[string]any {
	input := int64Of(usageIn["input_tokens"])
	output := int64Of(usageIn["output_tokens"])
	cacheRead := int64Of(usageIn["cache_read_input_tokens"])
	cacheCreate := int64Of(usageIn["cache_creation_input_tokens"])

	prompt := input + cacheRead + cacheCreate
	usage := map[string]any{
		"prompt_tokens":     prompt,
		"completion_tokens": output,
		"total_tokens":      prompt + output,

		"input_tokens":  input,
		"output_tokens": output,
	}
	if cacheRead > 0 || cacheCreate > 0 {
		details := map[string]any{}
		if cacheRead > 0 {
			details["cached_tokens"] = cacheRead
			usage["cache_read_input_tokens"] = cacheRead
		}
		if cacheCreate > 0 {
			details["cache_creation_tokens"] = cacheCreate
			usage["cache_creation_input_tokens"] = cacheCreate
		}
		usage["prompt_tokens_details"] = details
	}
	return usage
}

func mapAnthropicStop(s string) string {
	switch s {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	}
	if s == "" {
		return "stop"
	}
	return "stop"
}

func int64Of(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int:
		return int64(t)
	case int64:
		return t
	}
	return 0
}
