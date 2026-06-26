// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package streamutil

import "strings"

const (
	scopeAll       = "all"
	scopeToolCalls = "toolCalls"
)

const reasoningPlaceholder = " "

var ProviderRules = map[string]string{
	"deepseek": scopeAll,
}

type ModelRule struct {
	Match func(model string) bool
	Scope string
}

var ModelRules = []ModelRule{
	{Match: func(m string) bool { return strings.HasPrefix(m, "kimi-") }, Scope: scopeToolCalls},
	{Match: func(m string) bool { return strings.HasPrefix(m, "deepseek-") }, Scope: scopeAll},
}

func resolveScope(provider, model string) string {
	if s, ok := ProviderRules[strings.ToLower(provider)]; ok {
		return s
	}
	for _, r := range ModelRules {
		if r.Match(model) {
			return r.Scope
		}
	}
	return ""
}

func shouldInject(msg map[string]any, scope string) bool {
	if msg == nil || msg["role"] != "assistant" {
		return false
	}
	if rc, ok := msg["reasoning_content"].(string); ok && rc != "" {
		return false
	}
	if scope == scopeToolCalls {
		tc, _ := msg["tool_calls"].([]any)
		return len(tc) > 0
	}
	return true
}

func InjectReasoningContent(provider, model string, body map[string]any) map[string]any {
	scope := resolveScope(provider, model)
	if scope == "" || body == nil {
		return body
	}
	msgs, ok := body["messages"].([]any)
	if !ok {
		return body
	}
	out := make([]any, len(msgs))
	for i, raw := range msgs {
		m, ok := raw.(map[string]any)
		if !ok {
			out[i] = raw
			continue
		}
		if shouldInject(m, scope) {
			clone := make(map[string]any, len(m)+1)
			for k, v := range m {
				clone[k] = v
			}
			clone["reasoning_content"] = reasoningPlaceholder
			out[i] = clone
		} else {
			out[i] = m
		}
	}
	body["messages"] = out
	return body
}
