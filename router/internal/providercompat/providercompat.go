// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package providercompat

import "strings"

const (
	openAIPrefix    = "openai-compatible-"
	anthropicPrefix = "anthropic-compatible-"
	responsesSuffix = "-responses"

	defaultOpenAIBase    = "https://api.openai.com/v1"
	defaultAnthropicBase = "https://api.anthropic.com/v1"
)

func IsOpenAICompatible(provider string) bool {
	return strings.HasPrefix(provider, openAIPrefix)
}

func IsAnthropicCompatible(provider string) bool {
	return strings.HasPrefix(provider, anthropicPrefix)
}

func OpenAIAPIType(provider string) string {
	if !IsOpenAICompatible(provider) {
		return ""
	}
	if strings.Contains(provider, responsesSuffix) {
		return "responses"
	}
	return "chat"
}

func BuildOpenAICompatURL(baseURL, apiType string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = defaultOpenAIBase
	}
	switch apiType {
	case "responses":
		return base + "/responses"
	default:
		return base + "/chat/completions"
	}
}

func BuildAnthropicCompatURL(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = defaultAnthropicBase
	}
	return base + "/messages"
}

func ResolveFormat(provider string) string {
	if IsOpenAICompatible(provider) {
		if OpenAIAPIType(provider) == "responses" {
			return "openai-responses"
		}
		return "openai"
	}
	if IsAnthropicCompatible(provider) {
		return "anthropic"
	}
	return ""
}

func ResolveBaseURL(provider, baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if base != "" {
		return base
	}
	switch {
	case IsOpenAICompatible(provider):
		return defaultOpenAIBase
	case IsAnthropicCompatible(provider):
		return defaultAnthropicBase
	}
	return ""
}
