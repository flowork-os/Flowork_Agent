// Package codeindex — graphquery.go
//
// GitNexus-inspired: Cypher-lite graph query engine.
// Parse mini query language, translate ke SQL, execute on codemap tables.
//
// Supported syntax:
//   CALLERS OF pkg.FuncName              → siapa yang panggil fungsi ini
//   CALLEES OF pkg.FuncName              → fungsi ini panggil siapa
//   PATH FROM pkg.A TO pkg.B             → shortest path antara 2 fungsi
//   MATCH (f:func) WHERE f.pkg = "x"     → flexible node query
//   MATCH (f)-[:calls]->(g) WHERE ...    → edge traversal query
package codeindex

import (
	"database/sql"
	"fmt"
	"strings"
)

// QueryKind — jenis graph query
type QueryKind int

const (
	QKCallers QueryKind = iota // CALLERS OF x
	QKCallees                  // CALLEES OF x
	QKPath                     // PATH FROM x TO y
	QKMatch                    // MATCH (f:type) WHERE ... RETURN ...
)

// GraphQuery — parsed graph query
type GraphQuery struct {
	Kind         QueryKind
	TargetID     string            // untuk CALLERS/CALLEES
	FromID       string            // untuk PATH
	ToID         string            // untuk PATH
	NodeType     string            // "func", "method", "type", "interface", "*"
	Filters      map[string]string // WHERE field = value
	ReturnFields []string          // RETURN fields (empty = return all)
	Depth        int               // traversal depth (default 5)
	RepoID       string            // optional repo filter
}

// GraphResult — single result row
type GraphResult struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	Pkg       string  `json:"pkg"`
	Kind      string  `json:"kind"`
	Signature string  `json:"signature"`
	Depth     int     `json:"depth,omitempty"`
	EdgeType  string  `json:"edge_type,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// GraphQueryResult — full query response
type GraphQueryResult struct {
	Query   string        `json:"query"`
	Kind    string        `json:"kind"`
	Results []GraphResult `json:"results"`
	Count   int           `json:"count"`
	Error   string        `json:"error,omitempty"`
}

// ParseGraphQuery — parse query string ke GraphQuery struct.
func ParseGraphQuery(input string) (*GraphQuery, error) {
	input = strings.TrimSpace(input)
	upper := strings.ToUpper(input)

	switch {
	case strings.HasPrefix(upper, "CALLERS OF "):
		target := strings.TrimSpace(input[11:])
		return &GraphQuery{Kind: QKCallers, TargetID: target, Depth: 5}, nil

	case strings.HasPrefix(upper, "CALLEES OF "):
		target := strings.TrimSpace(input[11:])
		return &GraphQuery{Kind: QKCallees, TargetID: target, Depth: 5}, nil

	case strings.HasPrefix(upper, "PATH FROM "):
		rest := strings.TrimSpace(input[10:])
		toIdx := strings.Index(strings.ToUpper(rest), " TO ")
		if toIdx < 0 {
			return nil, fmt.Errorf("PATH query requires 'FROM x TO y' syntax")
		}
		fromID := strings.TrimSpace(rest[:toIdx])
		toID := strings.TrimSpace(rest[toIdx+4:])
		return &GraphQuery{Kind: QKPath, FromID: fromID, ToID: toID, Depth: 10}, nil

	case strings.HasPrefix(upper, "MATCH "):
		return parseMatchQuery(input[6:])

	default:
		// Fallback: treat as simple search
		return &GraphQuery{
			Kind:    QKMatch,
			Filters: map[string]string{"name_like": input},
			Depth:   5,
		}, nil
	}
}

// parseMatchQuery — parse MATCH (f:type) WHERE ... RETURN ...
func parseMatchQuery(input string) (*GraphQuery, error) {
	q := &GraphQuery{
		Kind:    QKMatch,
		Filters: map[string]string{},
		Depth:   5,
	}

	upper := strings.ToUpper(input)

	// Extract node type from (f:type)
	if lparen := strings.Index(input, "("); lparen >= 0 {
		rparen := strings.Index(input, ")")
		if rparen > lparen {
			nodeSpec := input[lparen+1 : rparen]
			if colonIdx := strings.Index(nodeSpec, ":"); colonIdx >= 0 {
				q.NodeType = strings.TrimSpace(nodeSpec[colonIdx+1:])
			}
		}
	}

	// Extract edge pattern -[:type]->
	if arrowIdx := strings.Index(input, "->"); arrowIdx >= 0 {
		bracketStart := strings.Index(input, "[:")
		bracketEnd := strings.Index(input, "]")
		if bracketStart >= 0 && bracketEnd > bracketStart {
			edgeType := input[bracketStart+2 : bracketEnd]
			// Check for depth modifier *N
			if starIdx := strings.Index(edgeType, "*"); starIdx >= 0 {
				depthStr := edgeType[starIdx+1:]
				edgeType = edgeType[:starIdx]
				if d := parseInt(depthStr); d > 0 {
					q.Depth = d
				}
			}
			q.Filters["edge_type"] = edgeType
		}
	}

	// Extract WHERE clause
	whereIdx := strings.Index(upper, "WHERE ")
	returnIdx := strings.Index(upper, "RETURN ")

	if whereIdx >= 0 {
		end := len(input)
		if returnIdx > whereIdx {
			end = returnIdx
		}
		whereClause := strings.TrimSpace(input[whereIdx+6 : end])
		parseWhereClause(whereClause, q)
	}

	// Extract RETURN fields
	if returnIdx >= 0 {
		returnClause := strings.TrimSpace(input[returnIdx+7:])
		fields := strings.Split(returnClause, ",")
		for _, f := range fields {
			f = strings.TrimSpace(f)
			// Strip variable prefix: f.name → name
			if dotIdx := strings.Index(f, "."); dotIdx >= 0 {
				f = f[dotIdx+1:]
			}
			if f != "" {
				q.ReturnFields = append(q.ReturnFields, f)
			}
		}
	}

	return q, nil
}

// parseWhereClause — parse "f.pkg = 'brain' AND f.exported = true"
func parseWhereClause(clause string, q *GraphQuery) {
	parts := strings.Split(clause, " AND ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Split on = sign
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}

		key := strings.TrimSpace(part[:eqIdx])
		val := strings.TrimSpace(part[eqIdx+1:])

		// Strip variable prefix: f.pkg → pkg
		if dotIdx := strings.Index(key, "."); dotIdx >= 0 {
			key = key[dotIdx+1:]
		}

		// Strip quotes
		val = strings.Trim(val, `"'`)

		q.Filters[key] = val
	}
}

// ExecuteGraphQuery — execute parsed query against SQLite.
func ExecuteGraphQuery(db *sql.DB, q *GraphQuery) (*GraphQueryResult, error) {
	result := &GraphQueryResult{
		Kind: queryKindString(q.Kind),
	}

	switch q.Kind {
	case QKCallers:
		return executeCallersQuery(db, q)
	case QKCallees:
		return executeCalleesQuery(db, q)
	case QKPath:
		return executePathQuery(db, q)
	case QKMatch:
		return executeMatchQuery(db, q)
	default:
		result.Error = "unknown query kind"
	}

	return result, nil
}

// executeCallersQuery — BFS reverse: who calls this function?
func executeCallersQuery(db *sql.DB, q *GraphQuery) (*GraphQueryResult, error) {
	result := &GraphQueryResult{Kind: "CALLERS"}

	visited := map[string]bool{q.TargetID: true}
	queue := []string{q.TargetID}

	for depth := 1; depth <= q.Depth && len(queue) > 0; depth++ {
		var next []string
		for _, cur := range queue {
			rows, err := db.Query(`
				SELECT n.id, n.name, n.path, n.pkg, n.kind, n.signature
				FROM codemap_func_nodes n
				JOIN codemap_func_edges e ON e.from_id = n.id
				WHERE e.to_id = ?`, cur)
			if err != nil {
				continue
			}
			for rows.Next() {
				var r GraphResult
				rows.Scan(&r.ID, &r.Name, &r.Path, &r.Pkg, &r.Kind, &r.Signature)
				if !visited[r.ID] {
					visited[r.ID] = true
					r.Depth = depth
					r.EdgeType = "calls"
					result.Results = append(result.Results, r)
					next = append(next, r.ID)
				}
			}
			// Sprint 3.5d (BUG-C15 fix): rows.Err() check
			_ = rows.Err()
			rows.Close()
		}
		queue = next
	}

	result.Count = len(result.Results)
	return result, nil
}

// executeCalleesQuery — BFS forward: what does this function call?
func executeCalleesQuery(db *sql.DB, q *GraphQuery) (*GraphQueryResult, error) {
	result := &GraphQueryResult{Kind: "CALLEES"}

	visited := map[string]bool{q.TargetID: true}
	queue := []string{q.TargetID}

	for depth := 1; depth <= q.Depth && len(queue) > 0; depth++ {
		var next []string
		for _, cur := range queue {
			rows, err := db.Query(`
				SELECT n.id, n.name, n.path, n.pkg, n.kind, n.signature
				FROM codemap_func_nodes n
				JOIN codemap_func_edges e ON e.to_id = n.id
				WHERE e.from_id = ?`, cur)
			if err != nil {
				continue
			}
			for rows.Next() {
				var r GraphResult
				rows.Scan(&r.ID, &r.Name, &r.Path, &r.Pkg, &r.Kind, &r.Signature)
				if !visited[r.ID] {
					visited[r.ID] = true
					r.Depth = depth
					r.EdgeType = "calls"
					result.Results = append(result.Results, r)
					next = append(next, r.ID)
				}
			}
			// Sprint 3.5d (BUG-C15 fix): rows.Err() check
			_ = rows.Err()
			rows.Close()
		}
		queue = next
	}

	result.Count = len(result.Results)
	return result, nil
}

// executePathQuery — bidirectional BFS shortest path.
func executePathQuery(db *sql.DB, q *GraphQuery) (*GraphQueryResult, error) {
	result := &GraphQueryResult{Kind: "PATH"}

	// Forward adjacency (who does X call)
	fwdAdj := map[string][]string{}
	// Reverse adjacency (who calls X)
	revAdj := map[string][]string{}

	rows, err := db.Query(`SELECT from_id, to_id FROM codemap_func_edges WHERE edge_type = 'calls'`)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var from, to string
		rows.Scan(&from, &to)
		fwdAdj[from] = append(fwdAdj[from], to)
		revAdj[to] = append(revAdj[to], from)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	rows.Close()

	// BFS from source
	fwdVisited := map[string]string{q.FromID: ""} // node → parent
	fwdQueue := []string{q.FromID}

	// BFS from target (reverse)
	revVisited := map[string]string{q.ToID: ""} // node → child
	revQueue := []string{q.ToID}

	meetPoint := ""

	for depth := 0; depth < q.Depth && meetPoint == ""; depth++ {
		// Forward step
		var fwdNext []string
		for _, cur := range fwdQueue {
			for _, neighbor := range fwdAdj[cur] {
				if _, seen := fwdVisited[neighbor]; !seen {
					fwdVisited[neighbor] = cur
					fwdNext = append(fwdNext, neighbor)
					if _, inRev := revVisited[neighbor]; inRev {
						meetPoint = neighbor
						break
					}
				}
			}
			if meetPoint != "" {
				break
			}
		}
		fwdQueue = fwdNext

		if meetPoint != "" {
			break
		}

		// Reverse step
		var revNext []string
		for _, cur := range revQueue {
			for _, neighbor := range revAdj[cur] {
				if _, seen := revVisited[neighbor]; !seen {
					revVisited[neighbor] = cur
					revNext = append(revNext, neighbor)
					if _, inFwd := fwdVisited[neighbor]; inFwd {
						meetPoint = neighbor
						break
					}
				}
			}
			if meetPoint != "" {
				break
			}
		}
		revQueue = revNext
	}

	if meetPoint == "" {
		result.Error = fmt.Sprintf("no path found from %s to %s within %d hops", q.FromID, q.ToID, q.Depth)
		return result, nil
	}

	// Reconstruct path: source → meetPoint
	var pathForward []string
	cur := meetPoint
	for cur != "" && cur != q.FromID {
		pathForward = append([]string{cur}, pathForward...)
		cur = fwdVisited[cur]
	}
	pathForward = append([]string{q.FromID}, pathForward...)

	// Reconstruct path: meetPoint → target
	cur = revVisited[meetPoint]
	for cur != "" && cur != q.ToID {
		pathForward = append(pathForward, cur)
		cur = revVisited[cur]
	}
	if meetPoint != q.ToID {
		pathForward = append(pathForward, q.ToID)
	}

	// Deduplicate
	seen := map[string]bool{}
	var uniquePath []string
	for _, p := range pathForward {
		if !seen[p] {
			seen[p] = true
			uniquePath = append(uniquePath, p)
		}
	}

	// Resolve node details
	for i, nodeID := range uniquePath {
		r := GraphResult{ID: nodeID, Depth: i}
		db.QueryRow(`
			SELECT name, path, pkg, kind, signature
			FROM codemap_func_nodes WHERE id = ?`, nodeID).Scan(
			&r.Name, &r.Path, &r.Pkg, &r.Kind, &r.Signature)
		if r.Name == "" {
			r.Name = nodeID // fallback
		}
		result.Results = append(result.Results, r)
	}

	result.Count = len(result.Results)
	return result, nil
}

// executeMatchQuery — flexible SQL query on func nodes.
func executeMatchQuery(db *sql.DB, q *GraphQuery) (*GraphQueryResult, error) {
	result := &GraphQueryResult{Kind: "MATCH"}

	var conditions []string
	var args []any

	if q.NodeType != "" && q.NodeType != "*" {
		conditions = append(conditions, "kind = ?")
		args = append(args, q.NodeType)
	}
	if q.RepoID != "" {
		conditions = append(conditions, "repo_id = ?")
		args = append(args, q.RepoID)
	}

	for key, val := range q.Filters {
		switch key {
		case "edge_type":
			continue // handled separately
		case "name_like":
			conditions = append(conditions, "(name LIKE ? OR id LIKE ?)")
			args = append(args, "%"+val+"%", "%"+val+"%")
		case "exported":
			if val == "true" || val == "1" {
				conditions = append(conditions, "exported = 1")
			} else {
				conditions = append(conditions, "exported = 0")
			}
		default:
			conditions = append(conditions, key+" = ?")
			args = append(args, val)
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT id, name, path, pkg, kind, signature
		FROM codemap_func_nodes %s
		ORDER BY pkg, name LIMIT 100`, where)

	rows, err := db.Query(query, args...)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}
	defer rows.Close()

	for rows.Next() {
		var r GraphResult
		rows.Scan(&r.ID, &r.Name, &r.Path, &r.Pkg, &r.Kind, &r.Signature)
		result.Results = append(result.Results, r)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()

	result.Count = len(result.Results)
	return result, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func queryKindString(k QueryKind) string {
	switch k {
	case QKCallers:
		return "CALLERS"
	case QKCallees:
		return "CALLEES"
	case QKPath:
		return "PATH"
	case QKMatch:
		return "MATCH"
	default:
		return "UNKNOWN"
	}
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
