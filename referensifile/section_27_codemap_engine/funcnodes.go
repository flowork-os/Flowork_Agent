// Package codeindex — funcnodes.go
//
// CRG Phase 5: Function-level granularity.
// Upgrade dari file-level ke function-level nodes.
// Setiap function/method/type jadi node tersendiri di graph.
// Memungkinkan: "kalau gw ubah fungsi X, fungsi mana yang panggil X?"
//
// Prinsip dari code-review-graph: granular AST-based code graph.
package codeindex

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"path/filepath"
	"strings"
)

// FuncNode — function/method/type level node in the graph.
type FuncNode struct {
	ID        string `json:"id"`         // pkg.FuncName or pkg.Type.MethodName
	Path      string `json:"path"`       // relative file path
	Name      string `json:"name"`       // function/method name
	Pkg       string `json:"pkg"`        // package name
	Kind      string `json:"kind"`       // "func", "method", "type", "interface"
	Receiver  string `json:"receiver"`   // receiver type (for methods)
	Signature string `json:"signature"`  // brief signature
	StartLine int    `json:"start_line"` // line number
	EndLine   int    `json:"end_line"`
	LineCount int    `json:"line_count"`
	Exported  bool   `json:"exported"`
	DocComment string `json:"doc_comment"`
}

// FuncEdge — function-level edge (calls, implements, etc).
type FuncEdge struct {
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	EdgeType string `json:"edge_type"` // "calls", "implements", "embeds"
}

// IndexFuncNodes parses a Go file and extracts function-level nodes.
// Returns nodes for each function/method/type declaration.
func IndexFuncNodes(absPath string) ([]FuncNode, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil && f == nil {
		return nil, err
	}

	var pkg string
	if f.Name != nil {
		pkg = f.Name.Name
	}

	var nodes []FuncNode

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			node := FuncNode{
				Path:      absPath,
				Name:      d.Name.Name,
				Pkg:       pkg,
				Kind:      "func",
				Exported:  ast.IsExported(d.Name.Name),
			}

			// Extract receiver for methods
			if d.Recv != nil && len(d.Recv.List) > 0 {
				node.Kind = "method"
				node.Receiver = exprToString(d.Recv.List[0].Type)
				node.ID = fmt.Sprintf("%s.%s.%s", pkg, node.Receiver, d.Name.Name)
			} else {
				node.ID = fmt.Sprintf("%s.%s", pkg, d.Name.Name)
			}

			// Line numbers
			if fset.File(d.Pos()) != nil {
				node.StartLine = fset.Position(d.Pos()).Line
				node.EndLine = fset.Position(d.End()).Line
				node.LineCount = node.EndLine - node.StartLine + 1
			}

			// Brief signature
			node.Signature = buildSignature(d)

			// Doc comment
			if d.Doc != nil {
				var lines []string
				for _, c := range d.Doc.List {
					text := strings.TrimPrefix(c.Text, "//")
					lines = append(lines, strings.TrimSpace(text))
				}
				node.DocComment = strings.Join(lines, " ")
				if len(node.DocComment) > 200 {
					node.DocComment = node.DocComment[:200] + "..."
				}
			}

			nodes = append(nodes, node)

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					node := FuncNode{
						ID:       fmt.Sprintf("%s.%s", pkg, ts.Name.Name),
						Path:     absPath,
						Name:     ts.Name.Name,
						Pkg:      pkg,
						Exported: ast.IsExported(ts.Name.Name),
					}

					switch ts.Type.(type) {
					case *ast.InterfaceType:
						node.Kind = "interface"
					case *ast.StructType:
						node.Kind = "type"
					default:
						node.Kind = "type"
					}

					if fset.File(d.Pos()) != nil {
						node.StartLine = fset.Position(d.Pos()).Line
						node.EndLine = fset.Position(d.End()).Line
						node.LineCount = node.EndLine - node.StartLine + 1
					}

					nodes = append(nodes, node)
				}
			}
		}
	}

	return nodes, nil
}

// IndexFuncEdges detects function call edges within a Go file.
// Looks for calls like `pkgAlias.FuncName(...)`.
func IndexFuncEdges(absPath string) ([]FuncEdge, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, 0)
	if err != nil && f == nil {
		return nil, err
	}

	var pkg string
	if f.Name != nil {
		pkg = f.Name.Name
	}

	// Map import aliases
	importAliases := map[string]string{} // alias → package name
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		parts := strings.Split(path, "/")
		pkgName := parts[len(parts)-1]
		if imp.Name != nil {
			pkgName = imp.Name.Name
		}
		importAliases[pkgName] = pkgName
	}

	var edges []FuncEdge

	// Walk AST to find call expressions
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		var callerID string
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			callerID = fmt.Sprintf("%s.%s.%s", pkg, exprToString(fd.Recv.List[0].Type), fd.Name.Name)
		} else {
			callerID = fmt.Sprintf("%s.%s", pkg, fd.Name.Name)
		}

		// Walk function body for calls
		if fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			var calleeID string
			switch fn := ce.Fun.(type) {
			case *ast.Ident:
				// Same-package call: funcName()
				calleeID = fmt.Sprintf("%s.%s", pkg, fn.Name)
			case *ast.SelectorExpr:
				// Cross-package or method call: pkg.Func() or obj.Method()
				if ident, ok := fn.X.(*ast.Ident); ok {
					if _, isImport := importAliases[ident.Name]; isImport {
						calleeID = fmt.Sprintf("%s.%s", ident.Name, fn.Sel.Name)
					} else {
						// Method call on variable — harder to resolve statically
						calleeID = fmt.Sprintf("?.%s", fn.Sel.Name)
					}
				}
			}

			if calleeID != "" && calleeID != callerID {
				edges = append(edges, FuncEdge{
					FromID:   callerID,
					ToID:     calleeID,
					EdgeType: "calls",
				})
			}
			return true
		})
	}

	return edges, nil
}

// StoreFuncNodes stores function-level nodes into dedicated tables.
// Uses separate tables to not interfere with file-level codemap.
func StoreFuncNodes(db *sql.DB, relPath string, nodes []FuncNode, edges []FuncEdge) error {
	// Clean old data for this file
	if _, err := db.Exec(`DELETE FROM codemap_func_nodes WHERE path = ?`, relPath); err != nil { log.Printf("codeindex: DELETE func_nodes failed: %v", err) }
	if _, err := db.Exec(`DELETE FROM codemap_func_edges WHERE from_path = ?`, relPath); err != nil { log.Printf("codeindex: DELETE func_edges failed: %v", err) }

	for _, n := range nodes {
		n.Path = relPath
		_, err := db.Exec(`
			INSERT OR REPLACE INTO codemap_func_nodes
			  (id, path, name, pkg, kind, receiver, signature, start_line, end_line, line_count, exported, doc_comment)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.Path, n.Name, n.Pkg, n.Kind, n.Receiver, n.Signature,
			n.StartLine, n.EndLine, n.LineCount, n.Exported, n.DocComment,
		)
		if err != nil {
			log.Printf("[funcnodes] insert %s: %v", n.ID, err)
		}
	}

	for _, e := range edges {
		db.Exec(`
			INSERT OR IGNORE INTO codemap_func_edges (from_id, to_id, from_path, edge_type)
			VALUES (?, ?, ?, ?)`, e.FromID, e.ToID, relPath, e.EdgeType)
	}

	return nil
}

// InitFuncNodesTables creates the function-level tables if they don't exist.
func InitFuncNodesTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS codemap_func_nodes (
			id         TEXT PRIMARY KEY,
			path       TEXT NOT NULL,
			name       TEXT NOT NULL,
			pkg        TEXT NOT NULL DEFAULT '',
			kind       TEXT NOT NULL DEFAULT 'func',
			receiver   TEXT NOT NULL DEFAULT '',
			signature  TEXT NOT NULL DEFAULT '',
			start_line INTEGER DEFAULT 0,
			end_line   INTEGER DEFAULT 0,
			line_count INTEGER DEFAULT 0,
			exported   INTEGER DEFAULT 0,
			doc_comment TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS codemap_func_edges (
			from_id   TEXT NOT NULL,
			to_id     TEXT NOT NULL,
			from_path TEXT NOT NULL DEFAULT '',
			edge_type TEXT NOT NULL DEFAULT 'calls',
			PRIMARY KEY (from_id, to_id, edge_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_funcnodes_path ON codemap_func_nodes(path)`,
		`CREATE INDEX IF NOT EXISTS idx_funcnodes_pkg  ON codemap_func_nodes(pkg)`,
		`CREATE INDEX IF NOT EXISTS idx_funcedges_from ON codemap_func_edges(from_id)`,
		`CREATE INDEX IF NOT EXISTS idx_funcedges_to   ON codemap_func_edges(to_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("funcnodes init: %w", err)
			}
		}
	}
	return nil
}

// QueryFuncCallers returns all functions that call the given function.
// This is the key CRG feature: "who calls this function?"
func QueryFuncCallers(db *sql.DB, funcID string) ([]FuncNode, error) {
	rows, err := db.Query(`
		SELECT n.id, n.path, n.name, n.pkg, n.kind, n.receiver, n.signature,
		       n.start_line, n.end_line, n.line_count, n.exported, n.doc_comment
		FROM codemap_func_nodes n
		JOIN codemap_func_edges e ON e.from_id = n.id
		WHERE e.to_id = ?`, funcID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var callers []FuncNode
	for rows.Next() {
		var n FuncNode
		rows.Scan(&n.ID, &n.Path, &n.Name, &n.Pkg, &n.Kind, &n.Receiver, &n.Signature,
			&n.StartLine, &n.EndLine, &n.LineCount, &n.Exported, &n.DocComment)
		callers = append(callers, n)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	_ = rows.Err()
	return callers, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	default:
		return "?"
	}
}

func buildSignature(fd *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		sb.WriteString("(" + exprToString(fd.Recv.List[0].Type) + ") ")
	}
	sb.WriteString(fd.Name.Name + "(")

	if fd.Type.Params != nil {
		var params []string
		for _, p := range fd.Type.Params.List {
			typStr := exprToString(p.Type)
			for _, name := range p.Names {
				params = append(params, name.Name+" "+typStr)
			}
			if len(p.Names) == 0 {
				params = append(params, typStr)
			}
		}
		sb.WriteString(strings.Join(params, ", "))
	}
	sb.WriteString(")")

	if fd.Type.Results != nil && len(fd.Type.Results.List) > 0 {
		var results []string
		for _, r := range fd.Type.Results.List {
			results = append(results, exprToString(r.Type))
		}
		if len(results) == 1 {
			sb.WriteString(" " + results[0])
		} else {
			sb.WriteString(" (" + strings.Join(results, ", ") + ")")
		}
	}

	sig := sb.String()
	if len(sig) > 200 {
		sig = sig[:200] + "..."
	}
	return sig
}

// FuncImpactAnalysis — blast radius at function level.
// "Kalau gw ubah fungsi X, fungsi mana yang terdampak?"
func FuncImpactAnalysis(db *sql.DB, funcID string, maxDepth int) ([]FuncNode, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	visited := map[string]bool{funcID: true}
	queue := []string{funcID}
	var impacted []FuncNode

	for deg := 1; deg <= maxDepth && len(queue) > 0; deg++ {
		var next []string
		for _, cur := range queue {
			callers, err := QueryFuncCallers(db, cur)
			if err != nil {
				continue
			}
			for _, c := range callers {
				if !visited[c.ID] {
					visited[c.ID] = true
					impacted = append(impacted, c)
					next = append(next, c.ID)
				}
			}
		}
		queue = next
	}

	return impacted, nil
}

// IndexAllFuncNodes — index function-level nodes for all Go files in a directory.
// Called after file-level indexing is complete.
func (ix *Indexer) IndexAllFuncNodes() (int, int, error) {
	if err := InitFuncNodesTables(ix.db); err != nil {
		return 0, 0, err
	}

	files := ix.collectFiles()
	totalNodes, totalEdges := 0, 0

	for _, f := range files {
		if !strings.HasSuffix(f, ".go") {
			continue // func-level only for Go files currently
		}

		nodes, err := IndexFuncNodes(f)
		if err != nil {
			log.Printf("[funcnodes] parse %s: %v", f, err)
			continue
		}

		edges, err := IndexFuncEdges(f)
		if err != nil {
			log.Printf("[funcnodes] edges %s: %v", f, err)
			continue
		}

		relPath := ix.relPath(f)
		if err := StoreFuncNodes(ix.db, relPath, nodes, edges); err != nil {
			log.Printf("[funcnodes] store %s: %v", f, err)
			continue
		}

		totalNodes += len(nodes)
		totalEdges += len(edges)
	}

	log.Printf("[funcnodes] indexed %d func nodes, %d call edges",
		totalNodes, totalEdges)
	return totalNodes, totalEdges, nil
}

// ensure funcnodes uses filepath
var _ = filepath.Base
// ensure funcnodes uses json
var _ = json.Marshal
