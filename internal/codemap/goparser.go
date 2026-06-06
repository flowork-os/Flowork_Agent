// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 27 phase 1 Go AST parser via stdlib go/ast. Extract
//   func + type + method nodes dengan line range. Phase 2 (call edges,
//   import edges, JS parser, language registry) → tambah file baru.
//
// goparser.go — Section 27 phase 1: minimal Go AST node extractor.

package codemap

import (
	"go/ast"
	"go/parser"
	"go/token"
)

// Node — generic minimal node info.
type Node struct {
	Type      string // "func" | "type" | "method"
	Name      string
	Signature string
	LineStart int
	LineEnd   int
	SizeLOC   int
}

// ParseGo — parse single .go file, return slice of Node.
func ParseGo(path string, content []byte) ([]Node, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var out []Node
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			ts := fset.Position(x.Pos())
			te := fset.Position(x.End())
			kind := "func"
			name := ""
			if x.Name != nil {
				name = x.Name.Name
			}
			if x.Recv != nil && len(x.Recv.List) > 0 {
				kind = "method"
			}
			out = append(out, Node{
				Type:      kind,
				Name:      name,
				Signature: shortSig(x),
				LineStart: ts.Line,
				LineEnd:   te.Line,
				SizeLOC:   te.Line - ts.Line + 1,
			})
		case *ast.TypeSpec:
			if x.Name == nil {
				return true
			}
			ts := fset.Position(x.Pos())
			te := fset.Position(x.End())
			out = append(out, Node{
				Type:      "type",
				Name:      x.Name.Name,
				Signature: "type " + x.Name.Name,
				LineStart: ts.Line,
				LineEnd:   te.Line,
				SizeLOC:   te.Line - ts.Line + 1,
			})
		}
		return true
	})
	return out, nil
}

// shortSig — minimal "func Name(...) (...)" string.
func shortSig(fn *ast.FuncDecl) string {
	if fn == nil {
		return ""
	}
	sig := "func "
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		sig += "(...)"
	}
	if fn.Name != nil {
		sig += fn.Name.Name
	}
	sig += "(...)"
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		sig += " (...)"
	}
	return sig
}
