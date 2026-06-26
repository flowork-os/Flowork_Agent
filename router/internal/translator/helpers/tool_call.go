// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
)

func NewToolCallID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "call_" + hex.EncodeToString(b)
}

func AnthropicToolUseToOpenAI(block map[string]any) map[string]any {
	id, _ := block["id"].(string)
	if id == "" {
		id = NewToolCallID()
	}
	name, _ := block["name"].(string)
	input, _ := block["input"]
	args, _ := json.Marshal(input)
	return map[string]any{
		"id":   id,
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": string(args),
		},
	}
}

func OpenAIToolCallToAnthropic(tc map[string]any) map[string]any {
	id, _ := tc["id"].(string)
	if id == "" {
		id = NewToolCallID()
	}
	fn, _ := tc["function"].(map[string]any)
	name, _ := fn["name"].(string)
	argsStr, _ := fn["arguments"].(string)
	var input any
	if argsStr != "" {
		_ = json.Unmarshal([]byte(argsStr), &input)
	}
	return map[string]any{
		"type":  "tool_use",
		"id":    id,
		"name":  name,
		"input": input,
	}
}

func GeminiFunctionCallToOpenAI(fc map[string]any) map[string]any {
	name, _ := fc["name"].(string)
	args, _ := fc["args"]
	argsJSON, _ := json.Marshal(args)
	return map[string]any{
		"id":   NewToolCallID(),
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": string(argsJSON),
		},
	}
}

func OpenAIToolCallToGemini(tc map[string]any) map[string]any {
	fn, _ := tc["function"].(map[string]any)
	name, _ := fn["name"].(string)
	argsStr, _ := fn["arguments"].(string)
	var args any
	if argsStr != "" {
		_ = json.Unmarshal([]byte(argsStr), &args)
	}
	return map[string]any{
		"functionCall": map[string]any{
			"name": name,
			"args": args,
		},
	}
}

func toolIDValid(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func sanitizeToolID(id string) string {
	out := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' {
			out = append(out, c)
		}
	}
	return string(out)
}

func deterministicToolCallID(msgIdx, callIdx int, name string) string {
	cleanName := sanitizeToolID(name)
	if cleanName == "" {
		cleanName = "tool"
	}
	if len(cleanName) > 40 {
		cleanName = cleanName[:40]
	}
	out := make([]byte, 0, 16+len(cleanName))
	out = append(out, "call_"...)
	out = appendInt(out, msgIdx)
	out = append(out, '_')
	out = appendInt(out, callIdx)
	out = append(out, '_')
	out = append(out, cleanName...)
	return string(out)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	if n < 0 {
		b = append(b, '-')
		n = -n
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(b, tmp[i:]...)
}

func EnsureToolCallIDs(body map[string]any) {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return
	}
	for i, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)

		if role == "assistant" {
			if tcs, ok := msg["tool_calls"].([]any); ok {
				for j, tc := range tcs {
					call, ok := tc.(map[string]any)
					if !ok {
						continue
					}
					id, _ := call["id"].(string)
					if !toolIDValid(id) {
						if clean := sanitizeToolID(id); clean != "" {
							call["id"] = clean
						} else {
							var name string
							if fn, ok := call["function"].(map[string]any); ok {
								name, _ = fn["name"].(string)
							}
							call["id"] = deterministicToolCallID(i, j, name)
						}
					}
					if _, has := call["type"]; !has {
						call["type"] = "function"
					}
					if fn, ok := call["function"].(map[string]any); ok {
						if args, has := fn["arguments"]; has {
							if _, isStr := args.(string); !isStr {
								raw, _ := json.Marshal(args)
								fn["arguments"] = string(raw)
							}
						}
					}
				}
			}
		}

		if role == "tool" {
			if id, ok := msg["tool_call_id"].(string); ok && !toolIDValid(id) {
				if clean := sanitizeToolID(id); clean != "" {
					msg["tool_call_id"] = clean
				} else {
					msg["tool_call_id"] = deterministicToolCallID(i, 0, "")
				}
			}
		}

		if blocks, ok := msg["content"].([]any); ok {
			for k, b := range blocks {
				block, ok := b.(map[string]any)
				if !ok {
					continue
				}
				typ, _ := block["type"].(string)
				switch typ {
				case "tool_use":
					id, _ := block["id"].(string)
					if !toolIDValid(id) {
						if clean := sanitizeToolID(id); clean != "" {
							block["id"] = clean
						} else {
							name, _ := block["name"].(string)
							block["id"] = deterministicToolCallID(i, k, name)
						}
					}
				case "tool_result":
					id, _ := block["tool_use_id"].(string)
					if !toolIDValid(id) {
						if clean := sanitizeToolID(id); clean != "" {
							block["tool_use_id"] = clean
						} else {
							block["tool_use_id"] = deterministicToolCallID(i, k, "")
						}
					}
				}
			}
		}
	}
}

func GetToolCallIDs(msg map[string]any) []string {
	var out []string
	if tcs, ok := msg["tool_calls"].([]any); ok {
		for _, tc := range tcs {
			if call, ok := tc.(map[string]any); ok {
				if id, _ := call["id"].(string); id != "" {
					out = append(out, id)
				}
			}
		}
	}
	if blocks, ok := msg["content"].([]any); ok {
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if typ, _ := block["type"].(string); typ == "tool_use" {
				if id, _ := block["id"].(string); id != "" {
					out = append(out, id)
				}
			}
		}
	}
	return out
}

func hasToolResults(msg map[string]any, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	seen := map[string]bool{}
	role, _ := msg["role"].(string)
	if role == "tool" {
		if id, _ := msg["tool_call_id"].(string); id != "" {
			seen[id] = true
		}
	}
	if blocks, ok := msg["content"].([]any); ok {
		for _, b := range blocks {
			block, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if typ, _ := block["type"].(string); typ == "tool_result" {
				if id, _ := block["tool_use_id"].(string); id != "" {
					seen[id] = true
				}
			}
		}
	}
	for _, id := range expected {
		if !seen[id] {
			return false
		}
	}
	return true
}

func FixMissingToolResponses(body map[string]any) {
	msgs, ok := body["messages"].([]any)
	if !ok {
		return
	}
	out := make([]any, 0, len(msgs)+2)
	for i := 0; i < len(msgs); i++ {
		out = append(out, msgs[i])
		msg, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		ids := GetToolCallIDs(msg)
		if len(ids) == 0 {
			continue
		}

		if i+1 < len(msgs) {
			if next, ok := msgs[i+1].(map[string]any); ok && hasToolResults(next, ids) {
				continue
			}
		}
		for _, id := range ids {
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": id,
				"content":      "",
			})
		}
	}
	body["messages"] = out
}
