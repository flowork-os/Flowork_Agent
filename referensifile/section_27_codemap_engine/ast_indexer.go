package factmemory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ASTNode merepresentasikan elemen logika kode (fungsi, tipe).
type ASTNode struct {
	Type      string `json:"type"` // "func", "type_struct", "type_interface"
	Name      string `json:"name"`
	Package   string `json:"package"`
	Filepath  string `json:"filepath"`
	Line      int    `json:"line"`
	Signature string `json:"signature,omitempty"`
}

// ASTIndex mewakili gudang memori indeks semantik.
type ASTIndex struct {
	Nodes []ASTNode `json:"nodes"`
}

// BuildIndex menyapu seluruh repositori Go untuk membangun AST semantik.
func BuildIndex(workspace string) error {
	fset := token.NewFileSet()
	var nodes []ASTNode

	err := filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			base := filepath.Base(path)
			// Hindari direktori sampah.
			if strings.HasPrefix(base, ".") || base == "state" || base == "node_modules" || base == "tools_temp" || base == "_sgvp" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		// Parse file tunggal
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		pkgName := f.Name.Name
		relPath, _ := filepath.Rel(workspace, path)

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				var sig bytes.Buffer
				printer.Fprint(&sig, fset, d.Type)

				prefix := ""
				if d.Recv != nil && len(d.Recv.List) > 0 {
					var recvType bytes.Buffer
					printer.Fprint(&recvType, fset, d.Recv.List[0].Type)
					prefix = fmt.Sprintf("(%s) ", recvType.String())
				}

				nodes = append(nodes, ASTNode{
					Type:      "func",
					Name:      prefix + d.Name.Name,
					Package:   pkgName,
					Filepath:  relPath,
					Line:      fset.Position(d.Pos()).Line,
					Signature: "func " + prefix + d.Name.Name + sig.String(),
				})

			case *ast.GenDecl:
				if d.Tok == token.TYPE {
					for _, spec := range d.Specs {
						typeSpec, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}

						nodeType := "type"
						if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
							nodeType = "type_struct"
						} else if _, isInterface := typeSpec.Type.(*ast.InterfaceType); isInterface {
							nodeType = "type_interface"
						}

						nodes = append(nodes, ASTNode{
							Type:     nodeType,
							Name:     typeSpec.Name.Name,
							Package:  pkgName,
							Filepath: relPath,
							Line:     fset.Position(typeSpec.Pos()).Line,
						})
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("ast walk error: %w", err)
	}

	index := ASTIndex{Nodes: nodes}

	outDir := filepath.Join(workspace, "state", "factmemory")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("ast index: mkdir %s: %w", outDir, err)
	}

	outFile := filepath.Join(outDir, "ast_index.json")
	b, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("ast index: marshal: %w", err)
	}

	return os.WriteFile(outFile, b, 0o644)
}
