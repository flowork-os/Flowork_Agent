package tools

// list_my_tools.go — tool yang return COMPLETE inventory dari rc174 capability
// matrix. Source of truth = tab GUI Tasking → Hak & Tools (wargacaps DB).
//
// rc185 fix (Ayah feedback): rc184 awal pake heuristic categorize tool name
// dari registry — itu BYPASS rc174 single source of truth. Setting tools ada
// di Tasking → Hak & Tools (GUI yang Ayah toggle), bukan di registry name
// pattern.
//
// Sekarang query langsung dari wargacaps:
//   - wargacaps.LoadFor(workspace, wargaName) → effective caps Ayah toggle
//   - wargacaps.AllToolCatalog() → kategori dari rc174 catalog
//   - Output match struktur GUI Tasking → Hak & Tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/wargacaps"
)

// ListMyToolsTool — return COMPLETE inventory dari rc174 capability matrix.
// Single source of truth: GUI Tasking → Hak & Tools (wargacaps DB).
type ListMyToolsTool struct {
	workspace string
}

// NewListMyToolsTool create tool. Workspace path required untuk
// wargacaps.LoadFor query DB.
func NewListMyToolsTool(workspace string) *ListMyToolsTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &ListMyToolsTool{workspace: workspace}
}

func (t *ListMyToolsTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "list_my_tools",
		Description: "Self-introspect: return COMPLETE inventory tools yang DIMILIKI warga ini (current persona) " +
			"dari rc174 capability matrix (source of truth: tab GUI Tasking → Hak & Tools). " +
			"PANGGIL TOOL INI saat user nanya 'sebutin tools', 'apa aja tools lo', 'list tools', " +
			"'tools apa aja yang lo punya', dll — JANGAN summarize manual top 10. " +
			"DISAMBIGUASI: tool ini = SELF tool inventory. JANGAN ke-mix dengan `cron_list` (list cron jobs), " +
			"`task_list` (list tasks), atau tool *_list lain — yang dimaksud user kalau bilang 'list tools' " +
			"= list_my_tools (nama PERSIS). Output deterministic dari DB capability Ayah toggle, BUKAN tool name heuristic.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"warga_name": map[string]any{
					"type":        "string",
					"description": "Optional. Default: nama warga lo (env FLOWORK_AGENT_HANDLE atau persona). Set kalau lo mau cek tool inventory warga lain.",
				},
			},
		},
	}
}

type argsListMyTools struct {
	WargaName string `json:"warga_name,omitempty"`
}

func (t *ListMyToolsTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var a argsListMyTools
	if len(inv.Arguments) > 0 {
		// W18 fix: log unmarshal err. Args optional → no fail, fallback to defaults.
		if err := json.Unmarshal(inv.Arguments, &a); err != nil {
			log.Printf("list_my_tools: arg parse err (using defaults): %v", err)
		}
	}

	// Resolve warga name: arg → env FLOWORK_AGENT_HANDLE → empty (caller provides).
	wargaName := strings.TrimSpace(a.WargaName)
	if wargaName == "" {
		wargaName = strings.TrimSpace(os.Getenv("FLOWORK_AGENT_HANDLE"))
	}
	if wargaName == "" {
		// Fallback ke "mr.flow" (post-pivot single warga, 2026-05-25 Mr.Dev konfirm).
		wargaName = "mr.flow"
	}

	// Query effective caps dari rc174 wargacaps (sama source dengan GUI Tasking).
	caps, err := wargacaps.LoadFor(t.workspace, wargaName)
	if err != nil {
		return Result{}, fmt.Errorf("list_my_tools: load caps for %q: %w", wargaName, err)
	}

	// Get rc174 catalog untuk kategori grouping.
	catalog := wargacaps.AllToolCatalog()

	// Compose output: per kategori, list tool yang enabled untuk warga ini.
	var sb strings.Builder
	totalEnabled := 0
	groupCount := 0
	type catSummary struct {
		Category string
		Enabled  []string
		Total    int
	}
	var summaries []catSummary

	for _, cat := range catalog {
		var enabled []string
		for _, tool := range cat.Tools {
			if caps.Has(tool) {
				enabled = append(enabled, tool)
			}
		}
		sort.Strings(enabled)
		summaries = append(summaries, catSummary{
			Category: cat.Category,
			Enabled:  enabled,
			Total:    len(cat.Tools),
		})
		if len(enabled) > 0 {
			groupCount++
			totalEnabled += len(enabled)
		}
	}

	sb.WriteString(fmt.Sprintf("Gue (warga **%s**) punya **%d tools enabled** di **%d kategori** "+
		"(rc174 capability matrix — source dari GUI Tasking → Hak & Tools).\n\n",
		wargaName, totalEnabled, groupCount))

	for _, s := range summaries {
		if len(s.Enabled) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%d/%d)\n", s.Category, len(s.Enabled), s.Total))
		sb.WriteString(strings.Join(s.Enabled, ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("---\n")
	sb.WriteString("📌 Mau toggle? Buka GUI: **Tasking → Hak & Tools** → cari card warga → centang/uncheck tool. ")
	sb.WriteString("Override per-individu (`*` ungu) menang atas role default.\n")

	return Result{
		ToolName: "list_my_tools",
		OK:       true,
		Output:   sb.String(),
		Metadata: map[string]any{
			"warga_name":     wargaName,
			"total_enabled":  totalEnabled,
			"category_count": groupCount,
			"source":         "rc174 wargacaps (GUI Tasking → Hak & Tools)",
		},
	}, nil
}
