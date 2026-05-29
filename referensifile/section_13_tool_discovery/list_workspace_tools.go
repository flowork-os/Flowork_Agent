package tools

// list_workspace_tools.go — Anti-bengkak workspace.
//
// Problem (Ayah report 2026-05-12): doktrin MANDIRI ajak warga "bikin tool
// sendiri di workspace kalau tool stuck". Tapi warga ngga inget tool yang
// udah dibikin sebelumnya → bikin terus-menerus → workspace bengkak +
// duplicate effort.
//
// Solusi: tool ini scan workspaces/<task>/tools/*.md di workspace warga,
// return inventory tool proposal + status. Warga WAJIB panggil tool ini
// SEBELUM tool_propose (doktrin step 0 ditanam di v7 training).
//
// Cross-cek: kalau warga bilang "gw butuh tool X", workflow:
//   1. list_my_tools — cek dari rc174 caps (registry global)
//   2. list_workspace_tools — cek dari workspace proposal (anti-dup)
//   3. brain_search 'tool X' — cek dari memory drawer
//   4. baru tool_propose (kalau benar2 tidak ada)

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

type ListWorkspaceToolsTool struct {
	workspace string
}

func NewListWorkspaceToolsTool(workspace string) *ListWorkspaceToolsTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &ListWorkspaceToolsTool{workspace: workspace}
}

func (t *ListWorkspaceToolsTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "list_workspace_tools",
		Description: "Self-introspect: list SEMUA tool proposal yang sudah lo (warga) atau warga lain bikin " +
			"di workspaces/<task>/tools/*.md. PANGGIL TOOL INI SEBELUM `tool_propose` — anti-bengkak workspace + " +
			"anti duplicate effort. Default scan SEMUA task workspace. " +
			"Optional arg `task` untuk filter 1 task tertentu. " +
			"Workflow saat tool stuck/missing: (1) list_my_tools cek rc174 caps → (2) list_workspace_tools cek " +
			"proposal lama → (3) brain_search 'tool X' cek memory → (4) baru tool_propose kalau memang absent.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Optional. Filter ke 1 task workspace. Kosong = scan semua.",
				},
				"name_contains": map[string]any{
					"type":        "string",
					"description": "Optional. Substring match nama tool (case-insensitive).",
				},
			},
		},
	}
}

type argsListWorkspaceTools struct {
	Task         string `json:"task,omitempty"`
	NameContains string `json:"name_contains,omitempty"`
}

type workspaceToolEntry struct {
	Task       string `json:"task"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Status     string `json:"status"` // proposed / approved / implemented / rejected
	ProposedBy string `json:"proposed_by,omitempty"`
	ProposedAt string `json:"proposed_at,omitempty"`
}

func (t *ListWorkspaceToolsTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var a argsListWorkspaceTools
	if len(inv.Arguments) > 0 {
		if err := json.Unmarshal(inv.Arguments, &a); err != nil {
			log.Printf("list_workspace_tools: arg parse err (using defaults): %v", err)
		}
	}

	wsRoot := filepath.Join(t.workspace, "workspaces")
	taskFilter := strings.TrimSpace(a.Task)
	nameFilter := strings.ToLower(strings.TrimSpace(a.NameContains))

	entries := []workspaceToolEntry{}

	// Iterate tasks (or single task if filter set)
	var tasks []string
	if taskFilter != "" {
		tasks = []string{taskFilter}
	} else {
		dirs, err := os.ReadDir(wsRoot)
		if err != nil {
			// No workspaces dir = no proposals yet, return empty success.
			return result(t.workspace, entries, taskFilter, nameFilter), nil
		}
		for _, d := range dirs {
			if d.IsDir() {
				tasks = append(tasks, d.Name())
			}
		}
	}
	sort.Strings(tasks)

	for _, task := range tasks {
		toolsDir := filepath.Join(wsRoot, task, "tools")
		files, err := os.ReadDir(toolsDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(f.Name(), ".md")
			if nameFilter != "" && !strings.Contains(strings.ToLower(name), nameFilter) {
				continue
			}
			full := filepath.Join(toolsDir, f.Name())
			meta := parseProposalMD(full)
			entries = append(entries, workspaceToolEntry{
				Task:       task,
				Name:       name,
				Path:       full,
				Status:     meta.status,
				ProposedBy: meta.proposedBy,
				ProposedAt: meta.proposedAt,
			})
		}
	}

	return result(t.workspace, entries, taskFilter, nameFilter), nil
}

type proposalMeta struct {
	status     string
	proposedBy string
	proposedAt string
}

func parseProposalMD(path string) proposalMeta {
	m := proposalMeta{status: "unknown"}
	raw, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	lines := strings.Split(string(raw), "\n")
	for i := 0; i < len(lines) && i < 30; i++ { // only check head section
		line := strings.TrimSpace(lines[i])
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "**status:**"):
			m.status = strings.TrimSpace(line[len("**Status:**"):])
		case strings.HasPrefix(lower, "**proposed by:**"):
			m.proposedBy = strings.TrimSpace(line[len("**Proposed by:**"):])
		case strings.HasPrefix(lower, "**proposed at:**"):
			m.proposedAt = strings.TrimSpace(line[len("**Proposed at:**"):])
		}
	}
	return m
}

func result(workspace string, entries []workspaceToolEntry, taskFilter, nameFilter string) Result {
	var sb strings.Builder
	if len(entries) == 0 {
		sb.WriteString("Tidak ada tool proposal di workspace")
		if taskFilter != "" {
			sb.WriteString(fmt.Sprintf(" task=%q", taskFilter))
		}
		if nameFilter != "" {
			sb.WriteString(fmt.Sprintf(" matching name=%q", nameFilter))
		}
		sb.WriteString(". Kalau lo butuh tool baru, drill protocol dulu (list_my_tools + tool_search + brain_search) sebelum tool_propose.")
		return Result{
			ToolName: "list_workspace_tools",
			OK:       true,
			Output:   sb.String(),
			Metadata: map[string]any{
				"count":         0,
				"task_filter":   taskFilter,
				"name_filter":   nameFilter,
				"workspace":     workspace,
			},
		}
	}

	sb.WriteString(fmt.Sprintf("Found **%d tool proposal** di workspace", len(entries)))
	if taskFilter != "" {
		sb.WriteString(fmt.Sprintf(" task=%q", taskFilter))
	}
	if nameFilter != "" {
		sb.WriteString(fmt.Sprintf(" matching name=%q", nameFilter))
	}
	sb.WriteString(":\n\n")

	// Group by task
	byTask := map[string][]workspaceToolEntry{}
	for _, e := range entries {
		byTask[e.Task] = append(byTask[e.Task], e)
	}
	tasks := make([]string, 0, len(byTask))
	for t := range byTask {
		tasks = append(tasks, t)
	}
	sort.Strings(tasks)
	for _, task := range tasks {
		es := byTask[task]
		sort.Slice(es, func(i, j int) bool { return es[i].Name < es[j].Name })
		sb.WriteString(fmt.Sprintf("### Task: `%s` (%d tool)\n", task, len(es)))
		for _, e := range es {
			sb.WriteString(fmt.Sprintf("- **%s** [%s]", e.Name, e.Status))
			if e.ProposedBy != "" {
				sb.WriteString(fmt.Sprintf(" — by %s", e.ProposedBy))
			}
			if e.ProposedAt != "" {
				sb.WriteString(fmt.Sprintf(" @ %s", e.ProposedAt))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")
	sb.WriteString("💡 Anti-bengkak: SEBELUM `tool_propose` ulang dengan name sama, cek list ini. Kalau name udah ada → edit MD file langsung atau pilih name beda. `tool_propose` dengan name duplicate akan REJECT (ERR_TOOL_ALREADY_PROPOSED).\n")

	return Result{
		ToolName: "list_workspace_tools",
		OK:       true,
		Output:   sb.String(),
		Metadata: map[string]any{
			"count":       len(entries),
			"task_filter": taskFilter,
			"name_filter": nameFilter,
			"tasks":       tasks,
			"workspace":   workspace,
		},
	}
}
