// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package streamutil

import (
	"encoding/json"
	"math"
)

const BufferTokens = 2000

const charsPerToken = 4

func AddBufferToUsage(usage map[string]any) map[string]any {
	if usage == nil {
		return usage
	}
	out := make(map[string]any, len(usage))
	for k, v := range usage {
		out[k] = v
	}

	if v, ok := toFiniteFloat(out["input_tokens"]); ok {
		out["input_tokens"] = v + BufferTokens
	}

	if v, ok := toFiniteFloat(out["prompt_tokens"]); ok {
		out["prompt_tokens"] = v + BufferTokens
	}

	if v, ok := toFiniteFloat(out["total_tokens"]); ok {
		out["total_tokens"] = v + BufferTokens
	} else {
		p, hasP := toFiniteFloat(out["prompt_tokens"])
		c, hasC := toFiniteFloat(out["completion_tokens"])
		if hasP && hasC {
			out["total_tokens"] = p + c
		}
	}
	return out
}

func NormalizeUsage(usage map[string]any) map[string]any {
	if usage == nil {
		return nil
	}
	out := map[string]any{}
	for _, k := range []string{
		"prompt_tokens", "completion_tokens", "total_tokens",
		"cache_read_input_tokens", "cache_creation_input_tokens",
		"cached_tokens", "reasoning_tokens",
	} {
		if v, ok := toFiniteFloat(usage[k]); ok {
			out[k] = v
		}
	}
	if d, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		out["prompt_tokens_details"] = d
	}
	if d, ok := usage["completion_tokens_details"].(map[string]any); ok {
		out["completion_tokens_details"] = d
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func FilterUsageForFormat(usage map[string]any, targetFormat string) map[string]any {
	if usage == nil {
		return nil
	}
	allowed, ok := allowedUsageFields[targetFormat]
	if !ok {
		return usage
	}
	out := map[string]any{}
	for _, k := range allowed {
		if v, present := usage[k]; present {
			out[k] = v
		}
	}
	return out
}

var allowedUsageFields = map[string][]string{
	FormatOpenAI: {
		"prompt_tokens", "completion_tokens", "total_tokens",
		"prompt_tokens_details", "completion_tokens_details",
		"estimated",
	},
	FormatResponses: {
		"input_tokens", "output_tokens", "total_tokens",
		"input_tokens_details", "output_tokens_details",
		"estimated",
	},
	FormatClaude: {
		"input_tokens", "output_tokens",
		"cache_read_input_tokens", "cache_creation_input_tokens",
		"estimated",
	},
	FormatGemini: {
		"promptTokenCount", "candidatesTokenCount", "totalTokenCount",
		"cachedContentTokenCount", "thoughtsTokenCount",
		"estimated",
	},
}

func EstimateInputTokens(body any) int {
	if body == nil {
		return 0
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(raw)) / float64(charsPerToken)))
}

func EstimateOutputTokens(contentLength int) int {
	if contentLength <= 0 {
		return 0
	}
	if contentLength < charsPerToken {
		return 1
	}
	return contentLength / charsPerToken
}

func EstimateUsage(body any, contentLength int, targetFormat string) map[string]any {
	in := EstimateInputTokens(body)
	out := EstimateOutputTokens(contentLength)
	return formatEstimatedUsage(in, out, targetFormat)
}

func formatEstimatedUsage(input, output int, targetFormat string) map[string]any {
	switch targetFormat {
	case FormatClaude:
		return AddBufferToUsage(map[string]any{
			"input_tokens":  float64(input),
			"output_tokens": float64(output),
			"estimated":     true,
		})
	case FormatGemini:
		total := input + output
		return AddBufferToUsage(map[string]any{
			"promptTokenCount":     float64(input),
			"candidatesTokenCount": float64(output),
			"totalTokenCount":      float64(total),
			"estimated":            true,
		})
	default:
		total := input + output
		return AddBufferToUsage(map[string]any{
			"prompt_tokens":     float64(input),
			"completion_tokens": float64(output),
			"total_tokens":      float64(total),
			"estimated":         true,
		})
	}
}

func HasValidUsage(usage map[string]any) bool {
	if usage == nil {
		return false
	}
	for _, k := range []string{
		"prompt_tokens", "completion_tokens", "total_tokens",
		"input_tokens", "output_tokens",
		"promptTokenCount", "candidatesTokenCount", "totalTokenCount",
	} {
		if v, ok := toFiniteFloat(usage[k]); ok && v > 0 {
			return true
		}
	}
	return false
}

func toFiniteFloat(v any) (float64, bool) {
	var f float64
	switch t := v.(type) {
	case float64:
		f = t
	case float32:
		f = float64(t)
	case int:
		f = float64(t)
	case int32:
		f = float64(t)
	case int64:
		f = float64(t)
	case uint:
		f = float64(t)
	case uint32:
		f = float64(t)
	case uint64:
		f = float64(t)
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}
