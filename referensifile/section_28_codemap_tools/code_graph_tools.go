// Package tools — code_graph_tools.go
//
// AI tools wrapping GitNexus G1/G3 functions untuk hemat token.
//
// Per arahan Ayah 2026-04-30: "AI selalu pakai ini agar hemat token".
// Ke-3 tools ini layered di atas CRG (`code_review_context`) — jadi AI
// punya pipeline lengkap:
//   1. code_review_context  → blast radius file-level (CRG)
//   2. code_graph_query     → callers/callees/path query (GitNexus G1)
//   3. code_flow_trace      → execution flow tracing (GitNexus G3)
//
// Result: AI baca codebase secara surgical, bukan grep/glob full repo.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/codeindex"
	"github.com/teetah2402/flowork/internal/provider"
)

// ── CodeGraphQueryTool (GitNexus G1) ─────────────────────────────────

type CodeGraphQueryTool struct{ workspace string }

type cgqArgs struct {
	Query string `json:"query" validate:"required"`
	Agent string `json:"agent,omitempty"`
}

func NewCodeGraphQueryTool(workspace string) *CodeGraphQueryTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &CodeGraphQueryTool{workspace: workspace}
}

func (t *CodeGraphQueryTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "code_graph_query",
		Description: "Cypher-lite graph query untuk explore call dependency. Pakai SEBELUM read code banyak. " +
			"4 mode: (1) `CALLERS OF pkg.Func` = siapa panggil fungsi ini, (2) `CALLEES OF pkg.Func` = " +
			"fungsi ini panggil siapa, (3) `PATH FROM a TO b` = shortest path antar fungsi, " +
			"(4) `MATCH (f:func) WHERE f.pkg = X AND f.exported = true RETURN f` = SQL-style. " +
			"Plain keyword (no prefix) = simple name search. Pakai untuk targeted exploration — hemat token.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Query string. Contoh: 'CALLERS OF codeindex.BuildReviewContext'.",
				},
				"agent": map[string]any{
					"type":        "string",
					"description": "Auto-injected by kernel.",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *CodeGraphQueryTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args cgqArgs
	if len(invocation.Arguments) > 0 {
		if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
			return Result{}, fmt.Errorf("decode: %w", err)
		}
	}
	if strings.TrimSpace(args.Query) == "" {
		return Result{}, fmt.Errorf("query required")
	}

	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}

	gq, err := codeindex.ParseGraphQuery(args.Query)
	if err != nil {
		return Result{}, fmt.Errorf("parse query: %w", err)
	}
	res, err := codeindex.ExecuteGraphQuery(db, gq)
	if err != nil {
		return Result{}, fmt.Errorf("execute: %w", err)
	}

	// Format output: human-readable list + metadata.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Query: %s (kind=%s)\n", res.Query, res.Kind))
	sb.WriteString(fmt.Sprintf("Results: %d\n\n", res.Count))
	for i, n := range res.Results {
		if i >= 30 {
			sb.WriteString(fmt.Sprintf("\n... (truncated at 30 of %d results)\n", res.Count))
			break
		}
		sb.WriteString(fmt.Sprintf("%2d. %s.%s [%s]", i+1, n.Pkg, n.Name, n.Kind))
		if n.Depth > 0 {
			sb.WriteString(fmt.Sprintf(" depth=%d", n.Depth))
		}
		if n.Path != "" {
			sb.WriteString(fmt.Sprintf(" @ %s", n.Path))
		}
		sb.WriteString("\n")
	}

	return Result{
		ToolName: "code_graph_query",
		OK:       true,
		Output:   sb.String(),
		Metadata: map[string]any{
			"query":    args.Query,
			"kind":     res.Kind,
			"count":    res.Count,
		},
	}, nil
}

// ── CodeFlowTraceTool (GitNexus G3) ──────────────────────────────────

type CodeFlowTraceTool struct{ workspace string }

type cftArgs struct {
	FuncID    string `json:"func_id,omitempty"`     // contoh: "main.main", "codeindex.IndexAll"
	Direction string `json:"direction,omitempty"`   // "forward" (default) atau "reverse"
	MaxDepth  int    `json:"max_depth,omitempty"`   // default 5
	Mode      string `json:"mode,omitempty"`        // "trace" (default) | "entry_points" | "path" | "processes"
	From      string `json:"from,omitempty"`        // untuk mode=path
	To        string `json:"to,omitempty"`          // untuk mode=path
	Agent     string `json:"agent,omitempty"`
}

func NewCodeFlowTraceTool(workspace string) *CodeFlowTraceTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &CodeFlowTraceTool{workspace: workspace}
}

func (t *CodeFlowTraceTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "code_flow_trace",
		Description: "Trace execution flow di codebase. 4 mode: (1) `mode=trace` (default) = DFS dari func_id sampai leaf " +
			"(direction=forward) atau root (direction=reverse), (2) `mode=path` = shortest path from→to, " +
			"(3) `mode=entry_points` = list main/init/handler/test funcs, (4) `mode=processes` = auto-discover " +
			"major execution flows. Pakai untuk understand call chain sebelum refactor / debug.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"trace", "path", "entry_points", "processes"},
					"description": "Default 'trace'.",
				},
				"func_id": map[string]any{
					"type":        "string",
					"description": "Func identifier 'pkg.FuncName'. Required untuk mode=trace.",
				},
				"direction": map[string]any{
					"type":        "string",
					"enum":        []string{"forward", "reverse"},
					"description": "Untuk mode=trace. Default 'forward'.",
				},
				"max_depth": map[string]any{
					"type":        "integer",
					"description": "BFS/DFS depth (default 5, max 15).",
				},
				"from": map[string]any{
					"type":        "string",
					"description": "Source func untuk mode=path.",
				},
				"to": map[string]any{
					"type":        "string",
					"description": "Target func untuk mode=path.",
				},
				"agent": map[string]any{
					"type":        "string",
					"description": "Auto-injected by kernel.",
				},
			},
		},
	}
}

func (t *CodeFlowTraceTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args cftArgs
	if len(invocation.Arguments) > 0 {
		if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
			return Result{}, fmt.Errorf("decode: %w", err)
		}
	}
	if args.Mode == "" {
		args.Mode = "trace"
	}
	depth := args.MaxDepth
	if depth <= 0 {
		depth = 5
	}
	if depth > 15 {
		depth = 15
	}

	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}

	switch args.Mode {
	case "trace":
		if strings.TrimSpace(args.FuncID) == "" {
			return Result{}, fmt.Errorf("func_id required for mode=trace")
		}
		var flow *codeindex.ExecutionFlow
		if strings.ToLower(args.Direction) == "reverse" {
			flow, err = codeindex.TraceReverse(db, args.FuncID, depth)
		} else {
			flow, err = codeindex.TraceForward(db, args.FuncID, depth)
		}
		if err != nil {
			return Result{}, err
		}
		return formatFlowResult(flow, args.Direction)

	case "path":
		if args.From == "" || args.To == "" {
			return Result{}, fmt.Errorf("from + to required for mode=path")
		}
		path, err := codeindex.FindPathBetween(db, args.From, args.To, depth)
		if err != nil {
			return Result{
				ToolName: "code_flow_trace",
				OK:       false,
				Output:   fmt.Sprintf("path %s → %s: %v", args.From, args.To, err),
			}, nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Path %s → %s (%d steps):\n", args.From, args.To, len(path)))
		for i, p := range path {
			sb.WriteString(fmt.Sprintf("  %d. %s.%s\n", i+1, p.Pkg, p.Name))
		}
		return Result{
			ToolName: "code_flow_trace",
			OK:       true,
			Output:   sb.String(),
			Metadata: map[string]any{"steps": len(path)},
		}, nil

	case "entry_points":
		eps, err := codeindex.FindEntryPoints(db)
		if err != nil {
			return Result{}, err
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Entry points: %d\n\n", len(eps)))
		for i, e := range eps {
			if i >= 30 {
				sb.WriteString(fmt.Sprintf("... (truncated at 30 of %d)\n", len(eps)))
				break
			}
			sb.WriteString(fmt.Sprintf("%2d. %s.%s [%s]\n", i+1, e.Pkg, e.Name, e.Kind))
		}
		return Result{
			ToolName: "code_flow_trace",
			OK:       true,
			Output:   sb.String(),
			Metadata: map[string]any{"count": len(eps)},
		}, nil

	case "processes":
		procs, err := codeindex.DetectProcesses(db, depth)
		if err != nil {
			return Result{}, err
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Major flows: %d\n\n", len(procs)))
		for i, p := range procs {
			if i >= 20 {
				sb.WriteString(fmt.Sprintf("... (truncated at 20 of %d)\n", len(procs)))
				break
			}
			sb.WriteString(fmt.Sprintf("%2d. entry=%s steps=%d depth=%d leaves=%d\n",
				i+1, p.EntryPoint, len(p.Steps), p.TotalDepth, len(p.LeafNodes)))
		}
		return Result{
			ToolName: "code_flow_trace",
			OK:       true,
			Output:   sb.String(),
			Metadata: map[string]any{"count": len(procs)},
		}, nil

	default:
		return Result{}, fmt.Errorf("unknown mode %q", args.Mode)
	}
}

func formatFlowResult(flow *codeindex.ExecutionFlow, dir string) (Result, error) {
	if dir == "" {
		dir = "forward"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Flow %s from %s\n", dir, flow.EntryPoint))
	sb.WriteString(fmt.Sprintf("  total_depth=%d steps=%d leaves=%d cycles=%d\n\n",
		flow.TotalDepth, len(flow.Steps), len(flow.LeafNodes), flow.CyclesHit))
	for i, s := range flow.Steps {
		if i >= 30 {
			sb.WriteString(fmt.Sprintf("\n... (truncated at 30 of %d steps)\n", len(flow.Steps)))
			break
		}
		indent := strings.Repeat("  ", s.Depth)
		sb.WriteString(fmt.Sprintf("%s%s.%s\n", indent, s.Pkg, s.Name))
	}
	return Result{
		ToolName: "code_flow_trace",
		OK:       true,
		Output:   sb.String(),
		Metadata: map[string]any{
			"total_depth": flow.TotalDepth,
			"step_count":  len(flow.Steps),
			"leaves":      len(flow.LeafNodes),
			"cycles":      flow.CyclesHit,
		},
	}, nil
}
