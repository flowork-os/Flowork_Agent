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
	fmt.Println("🛡️  [AUDITOR: THE PANIC SHIELD]")
	fmt.Println("Mendeteksi Type Assertion tanpa pengecekan (val := x.(T)), yang mana bisa membuat anak-anak (AI) tiba-tiba pingsan permanen/Crash.")

	fset := token.NewFileSet()
	found := 0

	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if strings.Contains(path, ".git") || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "tools_temp") || !strings.HasSuffix(path, ".go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return nil
		}

		ast.Inspect(node, func(n ast.Node) bool {
			// Deteksi Unsafe Type Assertion
			if assign, ok := n.(*ast.AssignStmt); ok {
				// Kalau v := x.(T) (cuma 1 variabel di sebelah kiri)
				if len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
					if _, isTypeAssert := assign.Rhs[0].(*ast.TypeAssertExpr); isTypeAssert {
						fmt.Printf("❌ [PANIC BOMB] %s:%d - Unsafe Type Assertion! Gunakan pola 'val, ok := x.(Type)'! Jika server luar mengirim tipe data salah, nyawa agen taruhannya (PANIC).\n", path, fset.Position(assign.Pos()).Line)
						found++
					}
				}
			}
			// Deteksi pemicu Panic manual
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
					fmt.Printf("❌ [SUICIDE SWITCH] %s:%d - Ditemukan perintah 'panic()'. Jangan pernah biarkan agen bunuh diri. Harusnya Kembalikan error (return err).\n", path, fset.Position(call.Pos()).Line)
					found++
				}
			}
			return true
		})
		return nil
	})

	if found == 0 {
		fmt.Println("✅ Bersih. Tidak ada satupun tombol bunuh diri di dalam kode anak-anak kita.")
	} else {
		fmt.Printf("\n⚠️ Potensi Ledakan Pingsan: %d\n", found)
	}
}
