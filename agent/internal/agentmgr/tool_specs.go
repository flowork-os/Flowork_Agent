// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentmgr

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/tools"
	"flowork-gui/internal/toolsidecar"
)

var coreExposedTools = []string{
	"file_read", "file_write", "file_list", "grep", "glob",
	"webfetch", "brain_search", "memory_get", "memory_set",
	"telegram_send", "tool_search", "now",

	"graph_recall", "instinct_recall", "brain_search_shared",

	"tool_create",
}

const maxExposedToolsDefault = 56

func maxExposedToolsLimit() int {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_MAX_EXPOSED_TOOLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 4 && n <= 200 {
			return n
		}
	}
	return maxExposedToolsDefault
}

var primaryExtraTools = []string{
	"PowerShell", "TaskCreate", "TaskUpdate", "TaskStop", "TaskOutput",
	"ScheduleWakeup", "Monitor", "SendUserFile", "StructuredOutput", "Workflow",

	"system_power",

	"codemap_search",

	"web_search",

	"task_list", "task_run",

	"browser_navigate", "browser_snapshot", "browser_click", "browser_type",
	"browser_upload", "browser_screenshot", "browser_set_cookies", "browser_eval", "browser_close",

	"cognitive_tensions", "cognitive_resolve",
}

func ToolSpecsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		id = defaultAgentID
	}

	isPrimary := IsPrimaryAgent(id)

	deferOn, exposeAll := resolveDeferPolicy(id, isPrimary)
	limit := maxExposedToolsLimit()
	if deferOn {

		limit = deferAnnounceMax
	}
	exposed := map[string]bool{}
	ordered := []string{}
	add := func(n string) {
		if exposed[n] || len(ordered) >= limit {
			return
		}

		if IsPrimaryOnlyTool(n) && !isPrimary {
			return
		}
		if _, ok := tools.Lookup(n); ok {
			exposed[n] = true
			ordered = append(ordered, n)
		}
	}

	for _, n := range coreExposedTools {
		add(n)
	}

	for _, n := range toolsidecar.NamesForAgent(id) {
		add(n)
	}

	if deferOn && exposeAll {

		for _, s := range tools.ListSummaries() {
			add(s.Name)
		}
	} else if store, err := openAgentStore(id); err == nil {
		if subs, serr := store.ListSubscriptions(); serr == nil {
			for _, s := range subs {
				if s.Source != "" && !strings.EqualFold(s.Source, "default") {
					add(s.ToolName)
				}
			}
		}
		store.Close()
	}

	always := map[string]bool{}
	if deferOn {
		for _, n := range coreExposedTools {
			always[n] = true
		}
		always[deferFetchTool] = true
		if isPrimary {
			for _, n := range primaryVitalTools {
				always[n] = true
			}
		}
	}

	specs := make([]map[string]any, 0, len(ordered))
	deferredLines := make([]string, 0)
	emitted := map[string]bool{}
	for _, n := range ordered {
		t, ok := tools.Lookup(n)
		if !ok {
			continue
		}
		if !deferOn || always[n] || isActiveDeferred(id, n) {

			specs = append(specs, toOpenAIToolSchema(t))
			emitted[n] = true
		} else {
			deferredLines = append(deferredLines, deferCatalogLine(t))
		}
	}

	if deferOn {

		if !emitted[deferFetchTool] {
			if t, ok := tools.Lookup(deferFetchTool); ok {
				specs = append(specs, toOpenAIToolSchema(t))
			}
		}
		if len(deferredLines) > 0 {
			injectDeferredCatalog(specs, deferredLines)
		}
	}

	resp := map[string]any{"tools": specs, "count": len(specs)}
	if deferOn {
		resp["deferred_count"] = len(deferredLines)
	}
	httpx.WriteJSON(w, resp)
}

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
			"name":        tools.DisplayName(t.Name()),
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
