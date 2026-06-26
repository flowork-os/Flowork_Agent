// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

func OpenAIFinishReason(reason string) string {
	switch reason {
	case "stop", "length", "tool_calls", "content_filter":
		return reason
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	}
	if reason == "" {
		return "stop"
	}
	return reason
}

func MergeSystemMessages(msgs []map[string]any) (string, []map[string]any) {
	var systemParts []string
	rest := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		role, _ := m["role"].(string)
		if role == "system" {
			if c, _ := m["content"].(string); c != "" {
				systemParts = append(systemParts, c)
			}
			continue
		}
		rest = append(rest, m)
	}
	merged := ""
	for i, p := range systemParts {
		if i > 0 {
			merged += "\n\n"
		}
		merged += p
	}
	return merged, rest
}

func EnsureLastUserMessage(msgs []map[string]any) []map[string]any {
	if len(msgs) == 0 {
		return []map[string]any{{"role": "user", "content": ""}}
	}
	if r, _ := msgs[len(msgs)-1]["role"].(string); r == "user" {
		return msgs
	}
	return append(msgs, map[string]any{"role": "user", "content": ""})
}
