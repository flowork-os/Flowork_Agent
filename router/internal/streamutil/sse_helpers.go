// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package streamutil

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const (
	FormatOpenAI    = "openai"
	FormatClaude    = "claude"
	FormatGemini    = "gemini"
	FormatResponses = "openai-responses"
	FormatOllama    = "ollama"
)

type SSEParsed struct {
	Object map[string]any
	Done   bool
}

func ParseSSELine(line, format string) *SSEParsed {
	if line == "" {
		return nil
	}

	if format == FormatOllama {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{") {
			return nil
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil
		}
		return &SSEParsed{Object: obj}
	}

	if line[0] != 'd' || !strings.HasPrefix(line, "data:") {
		return nil
	}
	data := strings.TrimSpace(line[len("data:"):])
	if data == "" {
		return nil
	}
	if data == "[DONE]" {
		return &SSEParsed{Done: true}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return nil
	}
	return &SSEParsed{Object: obj}
}

func HasValuableContent(chunk map[string]any, format string) bool {
	if chunk == nil {
		return false
	}
	switch format {
	case FormatOpenAI:
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			return false
		}
		choice, _ := choices[0].(map[string]any)
		if fr, _ := choice["finish_reason"].(string); fr != "" {
			return true
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			return false
		}
		if c, _ := delta["content"].(string); c != "" {
			return true
		}
		if r, _ := delta["reasoning_content"].(string); r != "" {
			return true
		}
		if tcs, _ := delta["tool_calls"].([]any); len(tcs) > 0 {
			return true
		}
		if role, _ := delta["role"].(string); role != "" {
			return true
		}
		return false

	case FormatClaude:
		typ, _ := chunk["type"].(string)
		if typ != "content_block_delta" {
			return true
		}
		delta, _ := chunk["delta"].(map[string]any)
		if delta == nil {
			return false
		}
		if t, _ := delta["text"].(string); t != "" {
			return true
		}
		if t, _ := delta["thinking"].(string); t != "" {
			return true
		}
		if p, _ := delta["partial_json"].(string); p != "" {
			return true
		}
		return false

	default:
		return true
	}
}

func FixInvalidID(chunk map[string]any) bool {
	if chunk == nil {
		return false
	}
	id, _ := chunk["id"].(string)
	if !isInvalidID(id) {
		return false
	}
	chunk["id"] = "chatcmpl-" + chooseFallbackID(chunk)
	return true
}

func isInvalidID(id string) bool {
	if id == "" {
		return false
	}
	if id == "chat" || id == "completion" {
		return true
	}
	return len(id) < 8
}

func chooseFallbackID(chunk map[string]any) string {
	if ef, ok := chunk["extend_fields"].(map[string]any); ok {
		if v, _ := ef["requestId"].(string); v != "" {
			return v
		}
		if v, _ := ef["traceId"].(string); v != "" {
			return v
		}
	}

	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
