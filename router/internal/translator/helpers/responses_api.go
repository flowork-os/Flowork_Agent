// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import (
	"encoding/json"
	"strings"
)

func ParseResponsesInput(raw json.RawMessage) []map[string]any {
	if len(raw) == 0 {
		return nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		return []map[string]any{{"role": "user", "content": s}}
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return []map[string]any{{"role": "user", "content": string(raw)}}
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {

		var s string
		if err := json.Unmarshal(item, &s); err == nil {
			out = append(out, map[string]any{"role": "user", "content": s})
			continue
		}
		var obj struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(item, &obj); err == nil {
			role := obj.Role
			if role == "" {
				role = "user"
			}
			out = append(out, map[string]any{"role": role, "content": FlattenAnthropicContent(jsonToAny(obj.Content))})
		}
	}
	return out
}

func EncodeResponsesOutput(text string) []map[string]any {
	return []map[string]any{
		{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "output_text", "text": text},
			},
		},
	}
}

func jsonToAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

func NormalizeResponsesInput(raw json.RawMessage) []map[string]any {
	placeholder := func(text string) []map[string]any {
		if text == "" {
			text = "..."
		}
		return []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": text},
			},
		}}
	}
	if len(raw) == 0 {
		return placeholder("")
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		txt := strings.TrimSpace(s)
		if txt == "" {
			return placeholder("")
		}
		return []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": txt},
			},
		}}
	}

	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		if len(arr) == 0 {
			return placeholder("")
		}
		return arr
	}
	return nil
}

func ConvertResponsesAPIFormat(body map[string]any) map[string]any {
	if _, has := body["input"]; !has {
		return body
	}
	result := make(map[string]any, len(body)+1)
	for k, v := range body {
		result[k] = v
	}

	messages := make([]map[string]any, 0)
	if instr, _ := body["instructions"].(string); instr != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instr})
	}

	rawInput, _ := json.Marshal(body["input"])
	items := NormalizeResponsesInput(rawInput)

	var pendingAssistant map[string]any
	var pendingToolResults []map[string]any
	flushAssistant := func() {
		if pendingAssistant != nil {
			messages = append(messages, pendingAssistant)
			pendingAssistant = nil
		}
	}
	flushToolResults := func() {
		for _, tr := range pendingToolResults {
			messages = append(messages, tr)
		}
		pendingToolResults = nil
	}

	for _, item := range items {
		itemType, _ := item["type"].(string)
		if itemType == "" {
			if _, hasRole := item["role"]; hasRole {
				itemType = "message"
			}
		}
		switch itemType {
		case "message":
			flushAssistant()
			flushToolResults()
			role, _ := item["role"].(string)
			messages = append(messages, map[string]any{
				"role":    role,
				"content": normaliseResponsesContent(item["content"]),
			})

		case "function_call":
			name, _ := item["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if pendingAssistant == nil {
				pendingAssistant = map[string]any{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": []map[string]any{},
				}
			}
			callID, _ := item["call_id"].(string)
			args, _ := item["arguments"].(string)
			tcs := pendingAssistant["tool_calls"].([]map[string]any)
			tcs = append(tcs, map[string]any{
				"id":   callID,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": args,
				},
			})
			pendingAssistant["tool_calls"] = tcs

		case "function_call_output":
			flushAssistant()
			callID, _ := item["call_id"].(string)
			output := item["output"]
			var content string
			if s, ok := output.(string); ok {
				content = s
			} else if output != nil {
				if raw, err := json.Marshal(output); err == nil {
					content = string(raw)
				}
			}
			pendingToolResults = append(pendingToolResults, map[string]any{
				"role":         "tool",
				"tool_call_id": callID,
				"content":      content,
			})

		case "reasoning":
			continue
		}
	}
	flushAssistant()
	flushToolResults()

	result["messages"] = messages
	for _, k := range []string{"input", "instructions", "include", "prompt_cache_key", "store", "reasoning"} {
		delete(result, k)
	}
	return result
}

func normaliseResponsesContent(raw any) any {
	parts, ok := raw.([]any)
	if !ok {
		return raw
	}
	out := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		obj, ok := p.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := obj["type"].(string)
		switch typ {
		case "input_text", "output_text":
			text, _ := obj["text"].(string)
			out = append(out, map[string]any{"type": "text", "text": text})
		case "input_image":
			url, _ := obj["image_url"].(string)
			if url == "" {
				url, _ = obj["file_id"].(string)
			}
			detail, _ := obj["detail"].(string)
			if detail == "" {
				detail = "auto"
			}
			out = append(out, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url":    url,
					"detail": detail,
				},
			})
		default:
			out = append(out, obj)
		}
	}
	return out
}
