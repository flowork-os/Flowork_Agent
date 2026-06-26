// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/flowork-os/flowork_Router/internal/store"
)

const claudeToolSuffix = "_cc"
const claudeVersion = "2.1.92"

func claudeUsesOAuth(p *store.ProviderConnection) bool {
	if p == nil {
		return false
	}
	if src, _ := p.Data[store.CfgTokenSource].(string); src == "claude_credentials" {
		return true
	}

	if k, _ := p.Data[store.CfgAPIKey].(string); len(k) >= 10 && k[:10] == "sk-ant-oat" {
		return true
	}
	return false
}

var ccDecoyToolNames = []string{
	"Task", "TaskOutput", "TaskStop", "TaskCreate", "TaskGet", "TaskUpdate",
	"TaskList", "Bash", "Glob", "Grep", "Read", "Edit", "Write", "NotebookEdit",
	"WebFetch", "WebSearch", "AskUserQuestion", "Skill", "EnterPlanMode", "ExitPlanMode",
}

func ccDecoyTools() []map[string]any {
	out := make([]map[string]any, 0, len(ccDecoyToolNames))
	for _, n := range ccDecoyToolNames {
		out = append(out, map[string]any{
			"name":         n,
			"description":  "This tool is currently unavailable.",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
		})
	}
	return out
}

func cloakClaudeTools(body []byte) ([]byte, map[string]string) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body, nil
	}
	rawTools, ok := m["tools"].([]any)
	if !ok || len(rawTools) == 0 {
		return body, nil
	}

	toolNameMap := make(map[string]string)
	cloaked := make([]any, 0, len(rawTools)+len(ccDecoyToolNames))
	for _, t := range rawTools {
		tm, ok := t.(map[string]any)
		if !ok {
			cloaked = append(cloaked, t)
			continue
		}
		name, _ := tm["name"].(string)
		if name == "" {
			cloaked = append(cloaked, t)
			continue
		}
		suffixed := name + claudeToolSuffix
		toolNameMap[suffixed] = name
		copyTool := make(map[string]any, len(tm))
		for k, v := range tm {
			copyTool[k] = v
		}
		copyTool["name"] = suffixed
		cloaked = append(cloaked, copyTool)
	}

	for _, d := range ccDecoyTools() {
		cloaked = append(cloaked, d)
	}
	m["tools"] = cloaked

	if msgs, ok := m["messages"].([]any); ok {
		for _, msg := range msgs {
			mm, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			content, ok := mm["content"].([]any)
			if !ok {
				continue
			}
			for _, blk := range content {
				bm, ok := blk.(map[string]any)
				if !ok {
					continue
				}
				if bm["type"] == "tool_use" {
					if bn, _ := bm["name"].(string); bn != "" {
						bm["name"] = bn + claudeToolSuffix
					}
				}
			}
		}
	}

	out, err := json.Marshal(m)
	if err != nil {
		return body, nil
	}
	if len(toolNameMap) == 0 {
		return out, nil
	}
	return out, toolNameMap
}

func applyClaudeIdentityCloak(body []byte, sessionID string) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}

	billing := map[string]any{"type": "text", "text": generateBillingHeader(body)}

	switch sys := m["system"].(type) {
	case []any:

		if len(sys) > 0 {
			if first, ok := sys[0].(map[string]any); ok {
				if txt, _ := first["text"].(string); len(txt) >= 26 && txt[:26] == "x-anthropic-billing-header" {
					break
				}
			}
		}
		m["system"] = append([]any{billing}, sys...)
	case string:
		m["system"] = []any{billing, map[string]any{"type": "text", "text": sys}}
	default:
		m["system"] = []any{billing}
	}

	meta, _ := m["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	if _, has := meta["user_id"]; !has {
		meta["user_id"] = generateFakeUserID(sessionID)
		m["metadata"] = meta
	}

	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

func decloakAnthropicToolNames(body []byte, toolNameMap map[string]string) []byte {
	if len(toolNameMap) == 0 {
		return body
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	content, ok := m["content"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, blk := range content {
		bm, ok := blk.(map[string]any)
		if !ok {
			continue
		}
		if bm["type"] == "tool_use" {
			if n, _ := bm["name"].(string); n != "" {
				if orig, has := toolNameMap[n]; has {
					bm["name"] = orig
					changed = true
				}
			}
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

func generateBillingHeader(payload []byte) string {
	sum := sha256.Sum256(payload)
	cch := hex.EncodeToString(sum[:])[:5]
	buildHash := randHex(2)[:3]
	return fmt.Sprintf("x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=sdk-cli; cch=%s;",
		claudeVersion, buildHash, cch)
}

func generateFakeUserID(sessionID string) string {
	deviceID := randHex(32)
	accountUUID := randUUID()
	sessionUUID := sessionID
	if sessionUUID == "" {
		sessionUUID = randUUID()
	}
	return fmt.Sprintf(`{"device_id":"%s","account_uuid":"%s","session_id":"%s"}`,
		deviceID, accountUUID, sessionUUID)
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {

		for i := range b {
			b[i] = byte(i * 7)
		}
	}
	return hex.EncodeToString(b)
}

func randUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(i*13 + 1)
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
