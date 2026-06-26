// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package router

import (
	"encoding/json"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	toolCallTagRe = regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
	leniNameRe    = regexp.MustCompile(`(?:"|')?name(?:"|')?\s*:\s*(?:"|')?([a-zA-Z0-9_.\-]+)(?:"|')?`)
	leniArgsRe    = regexp.MustCompile(`(?s)(?:"|')?(?:parameters|arguments)(?:"|')?\s*:\s*(\{.*\})`)
)

func toolcallRecoverEnabled() bool {
	return strings.TrimSpace(strings.ToLower(os.Getenv("FLOWORK_TOOLCALL_RECOVER"))) != "0"
}

func recoverTextToolCalls(resp *OpenAIResponse) {
	if resp == nil || !toolcallRecoverEnabled() {
		return
	}
	for i := range resp.Choices {
		msg := &resp.Choices[i].Message
		if hasNativeToolCalls(msg.ToolCalls) || !strings.Contains(msg.Content, "<tool_call>") {
			continue
		}
		type fnObj struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		type tcall struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function fnObj  `json:"function"`
		}
		var calls []tcall
		for j, m := range toolCallTagRe.FindAllStringSubmatch(msg.Content, -1) {
			name, args := parseToolCallInner(m[1])
			if name == "" {
				continue
			}
			calls = append(calls, tcall{ID: "call_recover_" + strconv.Itoa(j), Type: "function",
				Function: fnObj{Name: name, Arguments: args}})
		}

		msg.Content = stripToolCallTags(msg.Content)
		if len(calls) > 0 {
			if b, err := json.Marshal(calls); err == nil {
				msg.ToolCalls = b
				resp.Choices[i].FinishReason = "tool_calls"
				log.Printf("flow_router toolcall-recover: %d <tool_call> teks → native tool_calls (anti-bocor)", len(calls))
			}
		}
	}
}

func parseToolCallInner(inner string) (name, args string) {
	var raw struct {
		Name       string          `json:"name"`
		Arguments  json.RawMessage `json:"arguments"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if json.Unmarshal([]byte(inner), &raw) == nil && strings.TrimSpace(raw.Name) != "" {
		a := raw.Arguments
		if len(a) == 0 {
			a = raw.Parameters
		}
		if len(a) == 0 {
			a = json.RawMessage("{}")
		}
		return raw.Name, string(a)
	}

	nm := leniNameRe.FindStringSubmatch(inner)
	if nm == nil {
		return "", ""
	}
	args = "{}"
	if am := leniArgsRe.FindStringSubmatch(inner); am != nil && json.Valid([]byte(am[1])) {
		args = am[1]
	}
	return nm[1], args
}

func hasNativeToolCalls(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "[]"
}

func stripToolCallTags(s string) string {
	s = toolCallTagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "<tool_call>", "")
	s = strings.ReplaceAll(s, "</tool_call>", "")
	return strings.TrimSpace(s)
}
