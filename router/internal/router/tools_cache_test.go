package router

import (
	"encoding/json"
	"os"
	"testing"
)

// TestPromptCacheBreakpoints — default ON: system, tool terakhir, dan block
// terakhir pesan terakhir HARUS dapet cache_control ephemeral. OFF: ga ada satupun.
func TestPromptCacheBreakpoints(t *testing.T) {
	os.Unsetenv("FLOWORK_PROMPT_CACHE") // default = ON
	req := OpenAIRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 100,
		Messages: []OpenAIMessage{
			{Role: "system", Content: "persona statis gede"},
			{Role: "user", Content: "halo bro"},
		},
		Tools: json.RawMessage(`[
			{"type":"function","function":{"name":"a","description":"","parameters":{"type":"object"}}},
			{"type":"function","function":{"name":"b","description":"","parameters":{"type":"object"}}}
		]`),
	}
	raw, err := buildAnthropicToolBody(req)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// system → array block dgn cache_control
	sysArr, ok := body["system"].([]any)
	if !ok || len(sysArr) == 0 {
		t.Fatalf("system should be block array, got %T", body["system"])
	}
	if sysArr[0].(map[string]any)["cache_control"] == nil {
		t.Fatalf("system block missing cache_control")
	}
	// tool terakhir → cache_control
	tools := body["tools"].([]any)
	if tools[len(tools)-1].(map[string]any)["cache_control"] == nil {
		t.Fatalf("last tool missing cache_control")
	}
	// block terakhir pesan terakhir → cache_control
	msgs := body["messages"].([]any)
	lastContent := msgs[len(msgs)-1].(map[string]any)["content"].([]any)
	if lastContent[len(lastContent)-1].(map[string]any)["cache_control"] == nil {
		t.Fatalf("last message block missing cache_control")
	}
}

// TestPromptCacheFirstBlockOnly — DYNAMIC BOUNDARY: kalau ada >1 system message
// (stabil + volatile), cache_control cuma di blok PERTAMA (persona stabil); sisanya
// fresh. Ini yang bikin persona ke-cache tanpa volatile mbatalin prefix.
func TestPromptCacheFirstBlockOnly(t *testing.T) {
	os.Unsetenv("FLOWORK_PROMPT_CACHE") // ON
	req := OpenAIRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 100,
		Messages: []OpenAIMessage{
			{Role: "system", Content: "PERSONA STABIL (tier1+2)"},
			{Role: "system", Content: "VOLATILE: waktu + recall"},
			{Role: "user", Content: "halo"},
		},
	}
	raw, err := buildAnthropicToolBody(req)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	sysArr, ok := body["system"].([]any)
	if !ok || len(sysArr) != 2 {
		t.Fatalf("expected 2 system blocks, got %T len=%d", body["system"], len(sysArr))
	}
	if sysArr[0].(map[string]any)["cache_control"] == nil {
		t.Fatalf("first (stable) block must have cache_control")
	}
	if sysArr[1].(map[string]any)["cache_control"] != nil {
		t.Fatalf("second (volatile) block must NOT have cache_control")
	}
}

func TestPromptCacheOff(t *testing.T) {
	os.Setenv("FLOWORK_PROMPT_CACHE", "off")
	defer os.Unsetenv("FLOWORK_PROMPT_CACHE")
	req := OpenAIRequest{
		Model:     "claude-sonnet-4",
		MaxTokens: 100,
		Messages:  []OpenAIMessage{{Role: "system", Content: "x"}, {Role: "user", Content: "y"}},
	}
	raw, _ := buildAnthropicToolBody(req)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	// OFF: system tetap string biasa (bukan block array).
	if _, isArr := body["system"].([]any); isArr {
		t.Fatalf("cache OFF: system should stay plain string")
	}
}
