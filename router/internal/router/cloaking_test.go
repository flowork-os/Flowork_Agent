package router

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloakClaudeTools_SuffixAndDecoy(t *testing.T) {
	body := []byte(`{
		"model":"claude-x",
		"tools":[{"name":"my_search","description":"d","input_schema":{}}],
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"my_search","input":{}}]},
			{"role":"user","content":"hi"}
		]
	}`)
	out, toolMap := cloakClaudeTools(body)

	if toolMap["my_search_cc"] != "my_search" {
		t.Fatalf("toolMap should map suffixed→original, got %v", toolMap)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output not valid json: %v", err)
	}
	tools := m["tools"].([]any)
	// 1 client tool + 20 decoys.
	if len(tools) != 1+len(ccDecoyToolNames()) {
		t.Fatalf("expected %d tools, got %d", 1+len(ccDecoyToolNames()), len(tools))
	}
	first := tools[0].(map[string]any)
	if first["name"] != "my_search_cc" {
		t.Errorf("client tool not suffixed: %v", first["name"])
	}
	// decoy present + marked unavailable
	last := tools[len(tools)-1].(map[string]any)
	if last["description"] != "This tool is currently unavailable." {
		t.Errorf("decoy not marked unavailable: %v", last["description"])
	}
	// tool_use in messages renamed
	msgs := m["messages"].([]any)
	blk := msgs[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if blk["name"] != "my_search_cc" {
		t.Errorf("tool_use name not suffixed: %v", blk["name"])
	}
}

func TestCloakClaudeTools_NoToolsIsNoop(t *testing.T) {
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}]}`)
	out, toolMap := cloakClaudeTools(body)
	if toolMap != nil {
		t.Errorf("no-tools should yield nil map, got %v", toolMap)
	}
	if string(out) != string(body) {
		t.Errorf("no-tools should return body unchanged")
	}
}

func TestDecloakRoundTrip(t *testing.T) {
	body := []byte(`{"tools":[{"name":"Read_client","input_schema":{}}]}`)
	cloaked, toolMap := cloakClaudeTools(body)
	if toolMap["Read_client_cc"] != "Read_client" {
		t.Fatalf("bad toolMap %v", toolMap)
	}
	// Simulate a response that used the suffixed name.
	resp := []byte(`{"content":[{"type":"tool_use","id":"x","name":"Read_client_cc","input":{}}]}`)
	restored := decloakAnthropicToolNames(resp, toolMap)
	var m map[string]any
	_ = json.Unmarshal(restored, &m)
	name := m["content"].([]any)[0].(map[string]any)["name"]
	if name != "Read_client" {
		t.Errorf("decloak did not restore original name, got %v", name)
	}
	_ = cloaked
}

func TestApplyClaudeIdentityCloak_StringSystem(t *testing.T) {
	body := []byte(`{"model":"x","system":"you are helpful","messages":[]}`)
	out := applyClaudeIdentityCloak(body, "sess-123")
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	sys := m["system"].([]any)
	first := sys[0].(map[string]any)
	if txt, _ := first["text"].(string); !strings.HasPrefix(txt, "x-anthropic-billing-header") {
		t.Errorf("billing header not first system block: %v", first)
	}
	// original system preserved as second block
	second := sys[1].(map[string]any)
	if second["text"] != "you are helpful" {
		t.Errorf("original system lost: %v", second)
	}
	// fake user_id present + session aligned
	meta := m["metadata"].(map[string]any)
	uid, _ := meta["user_id"].(string)
	if !strings.Contains(uid, `"session_id":"sess-123"`) {
		t.Errorf("session_id not aligned in user_id: %v", uid)
	}
	if !strings.Contains(uid, `"device_id"`) || !strings.Contains(uid, `"account_uuid"`) {
		t.Errorf("user_id missing fields: %v", uid)
	}
}

func TestApplyClaudeIdentityCloak_Idempotent(t *testing.T) {
	body := []byte(`{"model":"x","messages":[]}`)
	once := applyClaudeIdentityCloak(body, "")
	twice := applyClaudeIdentityCloak(once, "")
	var m map[string]any
	_ = json.Unmarshal(twice, &m)
	sys := m["system"].([]any)
	// Only ONE billing header block, even after double-apply.
	count := 0
	for _, b := range sys {
		bm := b.(map[string]any)
		if txt, _ := bm["text"].(string); strings.HasPrefix(txt, "x-anthropic-billing-header") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("billing header injected %d times, want 1 (not idempotent)", count)
	}
}

func TestRandUUIDShape(t *testing.T) {
	u := randUUID()
	parts := strings.Split(u, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[4]) != 12 {
		t.Errorf("bad uuid shape: %s", u)
	}
	if parts[2][0] != '4' {
		t.Errorf("uuid not version 4: %s", u)
	}
}
