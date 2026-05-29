// Package factmemory — AST query layer (L-02 per hasil_audit_antygravity_opus_keren.md).
//
// ast_indexer.go sudah build declaration index (func/type). Query layer ini
// tambah call-site tracking supaya tool "dimana fungsi X dipanggil?" bisa
// dijawab tanpa `grep -r` manual. Output persist di state/factmemory/
// ast_calls.json — searchable + cheap-reload by downstream tools.
//
// Design prinsip:
//   - Separate file dari ast_indexer.go → zero change di existing fn BuildIndex.
//   - Pure walk-only. Tidak sentuh Protected Core.
//   - Query surface via pure function (no global state / singleton) supaya
//     testing mudah + thread-safe by construction.
package factmemory

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CallSite adalah satu lokasi pemanggilan fungsi di codebase.
type CallSite struct {
	Caller   string `json:"caller"`   // fully-qualified: "pkg.FuncName" atau "pkg.Type.Method"
	Callee   string `json:"callee"`   // target function name sebagaimana ditulis di source
	Package  string `json:"package"`  // package caller
	Filepath string `json:"filepath"` // workspace-relative
	Line     int    `json:"line"`     // 1-based line number call-site
}

// CallGraph bundles call-sites dengan lookup index by callee name
// (map dari callee → slice CallSite) untuk fast lookup.
type CallGraph struct {
	Calls []CallSite `json:"calls"`
}

// BuildCallGraph walks workspace sama seperti BuildIndex, tapi track
// CallExpr di setiap fungsi body. Output saved ke state/factmemory/
// ast_calls.json.
func BuildCallGraph(workspace string) (*CallGraph, error) {
	fset := token.NewFileSet()
	var calls []CallSite

	err := filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") || base == "state" || base == "node_modules" ||
				base == "tools_temp" || base == "_sgvp" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		pkgName := f.Name.Name
		relPath, _ := filepath.Rel(workspace, path)
		relPath = filepath.ToSlash(relPath)

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			caller := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				if starExpr, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
					if ident, ok := starExpr.X.(*ast.Ident); ok {
						caller = ident.Name + "." + fn.Name.Name
					}
				} else if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
					caller = ident.Name + "." + fn.Name.Name
				}
			}
			callerFQ := pkgName + "." + caller

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := calleeName(call.Fun)
				if callee == "" {
					return true
				}
				pos := fset.Position(call.Pos())
				calls = append(calls, CallSite{
					Caller:   callerFQ,
					Callee:   callee,
					Package:  pkgName,
					Filepath: relPath,
					Line:     pos.Line,
				})
				return true
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("call graph walk: %w", err)
	}

	graph := &CallGraph{Calls: calls}

	outDir := filepath.Join(workspace, "state", "factmemory")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return graph, fmt.Errorf("call graph: mkdir %s: %w", outDir, err)
	}
	outFile := filepath.Join(outDir, "ast_calls.json")
	b, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return graph, fmt.Errorf("call graph: marshal: %w", err)
	}
	if err := os.WriteFile(outFile, b, 0o644); err != nil {
		return graph, fmt.Errorf("call graph: write: %w", err)
	}
	return graph, nil
}

// calleeName extracts the target function name from a CallExpr Fun node.
// Handle three common cases:
//   - `foo()` → "foo"
//   - `pkg.Foo()` → "pkg.Foo"
//   - `obj.Method()` → "Method" (receiver ignored; name lookup intentional)
//
// For chained calls like `a.B().C()` we return "C" — the final call target.
// Complex expressions (type assertions, anonymous funcs) return "".
func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		// pkg.Foo or recv.Method — return ident of selector.
		if ident, ok := f.X.(*ast.Ident); ok {
			return ident.Name + "." + f.Sel.Name
		}
		return f.Sel.Name
	}
	return ""
}

// FindCallers returns all call-sites where `name` is the callee.
// Accepts plain name ("Foo") atau qualified ("pkg.Foo" / "Type.Method").
// Matching is exact string equal — caller should normalize input.
func (g *CallGraph) FindCallers(name string) []CallSite {
	if g == nil {
		return nil
	}
	var out []CallSite
	for _, c := range g.Calls {
		if c.Callee == name {
			out = append(out, c)
		}
		// Juga match kalau user kasih plain name vs qualified selector form.
		// e.g. query "Open" match "pkg.Open" too.
		if strings.HasSuffix(c.Callee, "."+name) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Filepath != out[j].Filepath {
			return out[i].Filepath < out[j].Filepath
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// LoadCallGraph reads persisted graph dari state/factmemory/ast_calls.json.
// Returns empty graph kalau file missing.
func LoadCallGraph(workspace string) (*CallGraph, error) {
	p := filepath.Join(workspace, "state", "factmemory", "ast_calls.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &CallGraph{}, nil
		}
		return nil, err
	}
	var g CallGraph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("call graph parse: %w", err)
	}
	return &g, nil
}

// LoadIndex reads persisted AST declaration index dari state/factmemory/
// ast_index.json (dibuat oleh BuildIndex). Returns empty kalau file missing.
func LoadIndex(workspace string) (*ASTIndex, error) {
	p := filepath.Join(workspace, "state", "factmemory", "ast_index.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &ASTIndex{}, nil
		}
		return nil, err
	}
	var idx ASTIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("ast index parse: %w", err)
	}
	return &idx, nil
}

// FindDecl returns declarations matching `name`. Match strategy:
//  1. Exact name match
//  2. Type.Method suffix match (e.g. "Open" match "Pool.Open")
func (idx *ASTIndex) FindDecl(name string) []ASTNode {
	if idx == nil {
		return nil
	}
	var out []ASTNode
	for _, n := range idx.Nodes {
		if n.Name == name {
			out = append(out, n)
		} else if strings.HasSuffix(n.Name, "."+name) || strings.HasSuffix(n.Name, ") "+name) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Filepath != out[j].Filepath {
			return out[i].Filepath < out[j].Filepath
		}
		return out[i].Line < out[j].Line
	})
	return out
}
