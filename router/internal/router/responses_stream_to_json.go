// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func ParseResponsesSSEToJSON(rawSSE []byte) map[string]any {
	state := struct {
		responseID string
		created    int64
		status     string
		usage      map[string]any
		items      map[int]map[string]any
	}{
		created: time.Now().Unix(),
		status:  "in_progress",
		usage:   map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
		items:   map[int]map[string]any{},
	}

	for _, block := range strings.Split(string(rawSSE), "\n\n") {
		processResponsesSSEBlock(block, &state.responseID, &state.created, &state.status, &state.usage, state.items)
	}

	maxIdx := -1
	for i := range state.items {
		if i > maxIdx {
			maxIdx = i
		}
	}
	output := make([]map[string]any, 0, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		if it, ok := state.items[i]; ok {
			output = append(output, it)
		} else {
			output = append(output, map[string]any{
				"type":    "message",
				"role":    "assistant",
				"content": []map[string]any{},
			})
		}
	}
	if state.responseID == "" {
		state.responseID = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}
	if state.status == "in_progress" {

		state.status = "completed"
	}
	return map[string]any{
		"id":         state.responseID,
		"object":     "response",
		"created_at": state.created,
		"status":     state.status,
		"output":     output,
		"usage":      state.usage,
	}
}

func processResponsesSSEBlock(block string, respID *string, created *int64, status *string, usage *map[string]any, items map[int]map[string]any) {
	var eventType, dataStr string
	for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(line[len("event:"):])
		} else if strings.HasPrefix(line, "data:") {
			dataStr = strings.TrimSpace(line[len("data:"):])
		}
	}
	if dataStr == "" || dataStr == "[DONE]" || eventType == "" {
		return
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return
	}

	switch eventType {
	case "response.created":
		if r, ok := data["response"].(map[string]any); ok {
			if id, _ := r["id"].(string); id != "" {
				*respID = id
			}
			if c, ok := r["created_at"].(float64); ok && c > 0 {
				*created = int64(c)
			}
		}

	case "response.output_item.done":
		idxF, _ := data["output_index"].(float64)
		if it, ok := data["item"].(map[string]any); ok {
			items[int(idxF)] = it
		}

	case "response.completed":
		*status = "completed"
		if r, ok := data["response"].(map[string]any); ok {
			if u, ok := r["usage"].(map[string]any); ok {
				*usage = u
			}
		}

	case "response.failed":
		*status = "failed"
	}
}
