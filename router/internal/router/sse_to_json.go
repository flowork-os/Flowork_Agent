// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type sseToolCallAcc struct {
	ID   string
	Name string
	Args string
}

func ParseSSEToOpenAIResponse(rawSSE []byte, fallbackModel string) map[string]any {
	if len(rawSSE) == 0 {
		return nil
	}
	var chunks []map[string]any
	for _, line := range strings.Split(string(rawSSE), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		payload := strings.TrimSpace(trimmed[len("data:"):])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var ch map[string]any
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			continue
		}
		chunks = append(chunks, ch)
	}
	if len(chunks) == 0 {
		return nil
	}

	first := chunks[0]
	var (
		content      strings.Builder
		reasoning    strings.Builder
		toolCalls    = map[int]*sseToolCallAcc{}
		finishReason = "stop"
		usage        map[string]any
	)

	for _, chunk := range chunks {
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			if u, ok := chunk["usage"].(map[string]any); ok {
				usage = u
			}
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if c, _ := delta["content"].(string); c != "" {
			content.WriteString(c)
		}
		if r, _ := delta["reasoning_content"].(string); r != "" {
			reasoning.WriteString(r)
		}
		if fr, _ := choice["finish_reason"].(string); fr != "" {
			finishReason = fr
		}
		if u, ok := chunk["usage"].(map[string]any); ok {
			usage = u
		}
		if tcs, ok := delta["tool_calls"].([]any); ok {
			for _, t := range tcs {
				tc, _ := t.(map[string]any)
				idxF, _ := tc["index"].(float64)
				idx := int(idxF)
				acc, has := toolCalls[idx]
				if !has {
					acc = &sseToolCallAcc{}
					toolCalls[idx] = acc
				}
				if id, _ := tc["id"].(string); id != "" {
					acc.ID = id
				}
				if fn, ok := tc["function"].(map[string]any); ok {
					if n, _ := fn["name"].(string); n != "" {
						acc.Name += n
					}
					if a, _ := fn["arguments"].(string); a != "" {
						acc.Args += a
					}
				}
			}
		}
	}

	message := map[string]any{"role": "assistant"}
	if content.Len() > 0 {
		message["content"] = content.String()
	} else if len(toolCalls) > 0 {
		message["content"] = nil
	} else {
		message["content"] = ""
	}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	if len(toolCalls) > 0 {
		idxs := make([]int, 0, len(toolCalls))
		for i := range toolCalls {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)
		out := make([]map[string]any, 0, len(idxs))
		for _, i := range idxs {
			acc := toolCalls[i]
			out = append(out, map[string]any{
				"id":   acc.ID,
				"type": "function",
				"function": map[string]any{
					"name":      acc.Name,
					"arguments": acc.Args,
				},
			})
		}
		message["tool_calls"] = out
	}

	id, _ := first["id"].(string)
	if id == "" {
		id = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	model, _ := first["model"].(string)
	if model == "" {
		model = fallbackModel
	}
	createdF, _ := first["created"].(float64)
	created := int64(createdF)
	if created == 0 {
		created = time.Now().Unix()
	}

	result := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
	}
	if usage != nil {
		result["usage"] = usage
	}
	return result
}
