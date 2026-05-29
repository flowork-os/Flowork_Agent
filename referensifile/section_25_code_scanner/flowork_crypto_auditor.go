//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	fmt.Println("🛡️  [AUDITOR: THE CRYPTO GUARDIAN]")
	fmt.Println("Mendeteksi penggunaan Kriptografi lemah (math/rand untuk rahasia, MD5/SHA1, Insecure TLS).")

	fset := token.NewFileSet()
	found := 0

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, ".git") || strings.HasSuffix(path, "_test.go") || !strings.HasSuffix(path, ".go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			// Deteksi math/rand
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					if id.Name == "rand" && (sel.Sel.Name == "Read" || sel.Sel.Name == "Intn" || sel.Sel.Name == "Seed") {
						// Periksa impor
						for _, imp := range node.Imports {
							if imp.Path.Value == `"math/rand"` {
								fmt.Printf("❌ [MATH/RAND] %s:%d - Jangan pakai `math/rand` untuk token/rahasia anak-anak. Pakai `crypto/rand`!\n", path, fset.Position(n.Pos()).Line)
								found++
							}
						}
					}
					if id.Name == "md5" || id.Name == "sha1" {
						fmt.Printf("❌ [WEAK HASH] %s:%d - MD5/SHA1 sudah kuno dan mudah dijebol hacker. Gunakan SHA256 atau SHA512.\n", path, fset.Position(n.Pos()).Line)
						found++
					}
				}
			}

			// Deteksi InsecureSkipVerify
			if kv, ok := n.(*ast.KeyValueExpr); ok {
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "InsecureSkipVerify" {
					if identDef, ok := kv.Value.(*ast.Ident); ok && identDef.Name == "true" {
						fmt.Printf("❌ [INSECURE TLS] %s:%d - Bahaya! InsecureSkipVerify: true akan membiarkan musuh membajak jalur koneksi (MITM).\n", path, fset.Position(n.Pos()).Line)
						found++
					}
				}
			}
			return true
		})
		return nil
	})

	if found == 0 {
		fmt.Println("✅ Bersih. Semua dinding kriptografi sekeras baja.")
	} else {
		fmt.Printf("\n⚠️ Temuan Kritis: %d\n", found)
	}
}
