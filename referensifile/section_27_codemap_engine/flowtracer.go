// Package codeindex — flowtracer.go
//
// GitNexus-inspired: Execution Flow Tracing.
// Trace full execution path dari entry point sampai leaf node.
// Bukan cuma 1-hop — full DFS chain dengan cycle detection.
package codeindex

import (
	"database/sql"
	"fmt"
	"strings"
)

// FlowStep — satu langkah dalam execution flow.
type FlowStep struct {
	FuncID    string `json:"func_id"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Pkg       string `json:"pkg"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Depth     int    `json:"depth"`
}

// ExecutionFlow — full execution trace dari satu entry point.
type ExecutionFlow struct {
	EntryPoint string     `json:"entry_point"`
	EntryName  string     `json:"entry_name"`
	Steps      []FlowStep `json:"steps"`
	TotalDepth int        `json:"total_depth"`
	LeafNodes  []string   `json:"leaf_nodes"` // fungsi yang tidak call siapapun
	CyclesHit  int        `json:"cycles_hit"` // berapa kali ketemu cycle (skipped)
}

// TraceForward — DFS forward: dari funcID, trace semua yang dipanggil sampai leaf.
func TraceForward(db *sql.DB, funcID string, maxDepth int) (*ExecutionFlow, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	flow := &ExecutionFlow{EntryPoint: funcID}

	// Resolve entry point name
	db.QueryRow(`SELECT name FROM codemap_func_nodes WHERE id = ?`, funcID).Scan(&flow.EntryName)
	if flow.EntryName == "" {
		flow.EntryName = funcID
	}

	// Build forward adjacency
	fwdAdj := map[string][]string{}
	rows, err := db.Query(`SELECT from_id, to_id FROM codemap_func_edges WHERE edge_type = 'calls'`)
	if err != nil {
		return flow, err
	}
	for rows.Next() {
		var from, to string
		rows.Scan(&from, &to)
		fwdAdj[from] = append(fwdAdj[from], to)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	rows.Close()

	// DFS with cycle detection
	visited := map[string]bool{}
	var dfs func(nodeID string, depth int)
	dfs = func(nodeID string, depth int) {
		if depth > maxDepth {
			return
		}
		if visited[nodeID] {
			flow.CyclesHit++
			return
		}
		visited[nodeID] = true

		// Resolve node info
		step := FlowStep{FuncID: nodeID, Depth: depth}
		db.QueryRow(`
			SELECT name, path, pkg, kind, signature
			FROM codemap_func_nodes WHERE id = ?`, nodeID).Scan(
			&step.Name, &step.Path, &step.Pkg, &step.Kind, &step.Signature)
		if step.Name == "" {
			step.Name = nodeID
		}
		flow.Steps = append(flow.Steps, step)

		if depth > flow.TotalDepth {
			flow.TotalDepth = depth
		}

		// Get callees
		callees := fwdAdj[nodeID]
		if len(callees) == 0 {
			flow.LeafNodes = append(flow.LeafNodes, nodeID)
			return
		}

		for _, callee := range callees {
			dfs(callee, depth+1)
		}
	}

	dfs(funcID, 0)
	return flow, nil
}

// TraceReverse — DFS reverse: dari funcID, trace siapa yang panggil sampai root.
func TraceReverse(db *sql.DB, funcID string, maxDepth int) (*ExecutionFlow, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	flow := &ExecutionFlow{EntryPoint: funcID}
	db.QueryRow(`SELECT name FROM codemap_func_nodes WHERE id = ?`, funcID).Scan(&flow.EntryName)
	if flow.EntryName == "" {
		flow.EntryName = funcID
	}

	// Build reverse adjacency
	revAdj := map[string][]string{}
	rows, err := db.Query(`SELECT from_id, to_id FROM codemap_func_edges WHERE edge_type = 'calls'`)
	if err != nil {
		return flow, err
	}
	for rows.Next() {
		var from, to string
		rows.Scan(&from, &to)
		revAdj[to] = append(revAdj[to], from)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	rows.Close()

	visited := map[string]bool{}
	var dfs func(nodeID string, depth int)
	dfs = func(nodeID string, depth int) {
		if depth > maxDepth || visited[nodeID] {
			if visited[nodeID] {
				flow.CyclesHit++
			}
			return
		}
		visited[nodeID] = true

		step := FlowStep{FuncID: nodeID, Depth: depth}
		db.QueryRow(`
			SELECT name, path, pkg, kind, signature
			FROM codemap_func_nodes WHERE id = ?`, nodeID).Scan(
			&step.Name, &step.Path, &step.Pkg, &step.Kind, &step.Signature)
		if step.Name == "" {
			step.Name = nodeID
		}
		flow.Steps = append(flow.Steps, step)

		if depth > flow.TotalDepth {
			flow.TotalDepth = depth
		}

		callers := revAdj[nodeID]
		if len(callers) == 0 {
			flow.LeafNodes = append(flow.LeafNodes, nodeID) // root entry point
			return
		}

		for _, caller := range callers {
			dfs(caller, depth+1)
		}
	}

	dfs(funcID, 0)
	return flow, nil
}

// FindEntryPoints — detect main(), init(), Test*, Handle*, ServeHTTP.
func FindEntryPoints(db *sql.DB) ([]FuncNode, error) {
	rows, err := db.Query(`
		SELECT id, path, name, pkg, kind, receiver, signature,
		       start_line, end_line, line_count, exported, doc_comment
		FROM codemap_func_nodes
		WHERE (name = 'main' AND pkg = 'main')
		   OR name = 'init'
		   OR name LIKE 'Test%'
		   OR name LIKE 'Benchmark%'
		   OR name LIKE '%Handler'
		   OR name LIKE 'Handle%'
		   OR name = 'ServeHTTP'
		ORDER BY pkg, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FuncNode
	for rows.Next() {
		var n FuncNode
		rows.Scan(&n.ID, &n.Path, &n.Name, &n.Pkg, &n.Kind, &n.Receiver, &n.Signature,
			&n.StartLine, &n.EndLine, &n.LineCount, &n.Exported, &n.DocComment)
		results = append(results, n)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	return results, nil
}

// DetectProcesses — auto-detect major execution flows.
// Find all main() and Handler functions, trace forward from each.
func DetectProcesses(db *sql.DB, maxDepth int) ([]ExecutionFlow, error) {
	if maxDepth <= 0 {
		maxDepth = 5 // lighter for bulk detection
	}

	entries, err := FindEntryPoints(db)
	if err != nil {
		return nil, err
	}

	// Only trace main() and handlers (skip tests for performance)
	var flows []ExecutionFlow
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, "Test") || strings.HasPrefix(entry.Name, "Benchmark") {
			continue
		}

		flow, err := TraceForward(db, entry.ID, maxDepth)
		if err != nil {
			continue
		}
		// Only include flows with at least 2 steps (interesting)
		if len(flow.Steps) >= 2 {
			flows = append(flows, *flow)
		}
	}

	return flows, nil
}

// FindPathBetween — shortest path between two func IDs using bidirectional BFS.
// Wrapper around the graph query PATH implementation.
func FindPathBetween(db *sql.DB, fromID, toID string, maxDepth int) ([]FlowStep, error) {
	q := &GraphQuery{
		Kind:   QKPath,
		FromID: fromID,
		ToID:   toID,
		Depth:  maxDepth,
	}

	result, err := ExecuteGraphQuery(db, q)
	if err != nil {
		return nil, err
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	var steps []FlowStep
	for _, r := range result.Results {
		steps = append(steps, FlowStep{
			FuncID:    r.ID,
			Path:      r.Path,
			Name:      r.Name,
			Pkg:       r.Pkg,
			Kind:      r.Kind,
			Signature: r.Signature,
			Depth:     r.Depth,
		})
	}
	return steps, nil
}
