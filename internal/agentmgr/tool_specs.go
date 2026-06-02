// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-02
// Reason: Fase 0 — endpoint OpenAI function-schema (core ~13 + manual subs,
//   cap 25, BUKAN 106 = anti over-prompt). E2E verified (Mr.Flow file_write+read).
//
// tool_specs.go — Fase 0 (tool-calling loop): endpoint yang balikin tools yang
// di-EXPOSE ke LLM dalam format OpenAI function-schema. Host yang bangun schema
// (punya registry + subscription); WASM agent tinggal fetch + forward ke LLM.
//
// ANTI OVER-PROMPT (akar refactor 11×): yang di-expose CUMA core set (~13),
// BUKAN 106. Sisanya tetep di registry, dipanggil via `tool_search` on-demand.
// Fase 2: per-agent exposed selection (sekarang core + tool yang di-subscribe
// MANUAL via popup).

package agentmgr

import (
	"net/http"
	"strings"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/tools"
)

// coreExposedTools — SELALU di-expose ke LLM (cover kebutuhan umum). Kecil =
// prompt kecil. Sisanya via tool_search.
var coreExposedTools = []string{
	"file_read", "file_write", "file_list", "bash", "grep", "glob",
	"webfetch", "brain_search", "memory_get", "memory_set",
	"telegram_send", "tool_search", "now",
}

const maxExposedTools = 25

// ToolSpecsHandler — GET /api/agents/tools/specs?id=<agent>
// Return {tools: [<openai function schema>...], count}. Loopback-only (dipanggil
// WASM agent sendiri).
func ToolSpecsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		id = defaultAgentID
	}

	exposed := map[string]bool{}
	ordered := []string{}
	add := func(n string) {
		if exposed[n] || len(ordered) >= maxExposedTools {
			return
		}
		if _, ok := tools.Lookup(n); ok {
			exposed[n] = true
			ordered = append(ordered, n)
		}
	}
	// 1. core set (selalu).
	for _, n := range coreExposedTools {
		add(n)
	}
	// 2. tool yang di-subscribe MANUAL (owner pilih di popup) — di luar default seed.
	if store, err := openAgentStore(id); err == nil {
		if subs, serr := store.ListSubscriptions(); serr == nil {
			for _, s := range subs {
				if s.Source != "" && !strings.EqualFold(s.Source, "default") {
					add(s.ToolName)
				}
			}
		}
		store.Close()
	}

	specs := make([]map[string]any, 0, len(ordered))
	for _, n := range ordered {
		if t, ok := tools.Lookup(n); ok {
			specs = append(specs, toOpenAIToolSchema(t))
		}
	}
	httpx.WriteJSON(w, map[string]any{"tools": specs, "count": len(specs)})
}

// toOpenAIToolSchema — konversi tools.Schema → OpenAI function-calling schema.
func toOpenAIToolSchema(t tools.Tool) map[string]any {
	sc := t.Schema()
	props := map[string]any{}
	required := []string{}
	for _, p := range sc.Params {
		props[p.Name] = map[string]any{
			"type":        jsonSchemaType(p.Type),
			"description": p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name(),
			"description": sc.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": props,
				"required":   required,
			},
		},
	}
}

func jsonSchemaType(pt tools.ParamType) string {
	switch pt {
	case tools.ParamInt:
		return "integer"
	case tools.ParamFloat:
		return "number"
	case tools.ParamBool:
		return "boolean"
	case tools.ParamArray:
		return "array"
	case tools.ParamObject:
		return "object"
	default:
		return "string"
	}
}
