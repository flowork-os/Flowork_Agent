// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import "strings"

const reasoningPlaceholder = " "

type reasoningScope int

const (
	scopeNone reasoningScope = iota
	scopeAll
	scopeToolCalls
)

func pickReasoningScope(provider, model string) reasoningScope {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))

	if provider == "deepseek" {
		return scopeAll
	}

	switch {
	case strings.HasPrefix(model, "deepseek-"):
		return scopeAll
	case strings.HasPrefix(model, "kimi-"):
		return scopeToolCalls
	}
	return scopeNone
}

func InjectReasoningContent(provider, model string, body map[string]any) {
	scope := pickReasoningScope(provider, model)
	if scope == scopeNone {
		return
	}
	msgs, ok := body["messages"].([]any)
	if !ok {
		return
	}
	for _, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		if rc, _ := msg["reasoning_content"].(string); rc != "" {
			continue
		}
		if scope == scopeToolCalls {
			tcs, _ := msg["tool_calls"].([]any)
			if len(tcs) == 0 {
				continue
			}
		}
		msg["reasoning_content"] = reasoningPlaceholder
	}
}

type deepseekAlias struct {
	ThinkingType    string
	ReasoningEffort string
}

var deepseekV4ProAliases = map[string]deepseekAlias{
	"deepseek-v4-pro-max":  {ThinkingType: "enabled", ReasoningEffort: "max"},
	"deepseek-v4-pro-none": {ThinkingType: "disabled", ReasoningEffort: ""},
}

func ApplyDeepSeekV4ProAlias(provider string, body map[string]any) {
	if strings.ToLower(provider) != "deepseek" {
		return
	}
	model, _ := body["model"].(string)
	alias, ok := deepseekV4ProAliases[strings.ToLower(model)]
	if !ok {
		return
	}
	body["model"] = "deepseek-v4-pro"
	extra, _ := body["extra_body"].(map[string]any)
	if extra == nil {
		extra = map[string]any{}
		body["extra_body"] = extra
	}
	thinking, _ := extra["thinking"].(map[string]any)
	if thinking == nil {
		thinking = map[string]any{}
		extra["thinking"] = thinking
	}
	thinking["type"] = alias.ThinkingType
	if alias.ReasoningEffort != "" {
		body["reasoning_effort"] = alias.ReasoningEffort
	} else {
		delete(body, "reasoning_effort")
	}
}
