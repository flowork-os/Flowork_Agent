// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import "strings"

var ModelMaxTokens = map[string]int{
	"claude-opus":       32_000,
	"claude-sonnet":     8192,
	"claude-haiku":      4096,
	"gpt-4o":            16_384,
	"gpt-4-turbo":       4096,
	"gpt-3.5-turbo":     4096,
	"gemini-1.5-pro":    8192,
	"gemini-2.5-pro":    65_536,
	"gemini-2.5-flash":  65_536,
	"gemini-3":          65_536,
	"deepseek-chat":     8192,
	"deepseek-reasoner": 8192,
	"qwen":              8192,
	"kimi":              8192,
}

const DefaultMaxTokens = 4096

func MaxTokensForModel(model string) int {
	if model == "" {
		return DefaultMaxTokens
	}
	best := ""
	for prefix := range ModelMaxTokens {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best != "" {
		return ModelMaxTokens[best]
	}
	return DefaultMaxTokens
}

func ResolveMaxTokens(explicit int, model string) int {
	if explicit > 0 {
		return explicit
	}
	return MaxTokensForModel(model)
}

const MinMaxTokensWithTools = 32000

func AdjustMaxTokens(maxTokens int, hasTools bool, thinkingBudget int) int {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	if hasTools && maxTokens < MinMaxTokensWithTools {
		maxTokens = MinMaxTokensWithTools
	}
	if thinkingBudget > 0 && maxTokens <= thinkingBudget {
		maxTokens = thinkingBudget + 1024
	}
	return maxTokens
}
