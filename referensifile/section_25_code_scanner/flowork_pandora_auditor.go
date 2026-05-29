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

type PandoraVuln struct {
	VulnType string
	File     string
	Line     int
	Message  string
}

var pandoraFindings []PandoraVuln

func main() {
	fmt.Println("📦 [ANTIGRAVITY PANDORA'S BOX] Kamu yang minta, aku bukain kotaknya!")
	fmt.Println("Mendeteksi celah dimensi ke-4: TOCTOU (Time-of-Check to Time-of-Use), Global State Poisoning, dan Reflect Panics.")
	fmt.Println("Hanya Dewa Arsitektur Sistem yang mikirin bug gatel kayak gini. Let's roll...")

	rootDir := "."
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (strings.Contains(path, ".git") || strings.Contains(path, "tools_temp")) {
			return filepath.SkipDir
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			scanPandora(path)
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("\n[😱] TUNTAS! Kotak Pandora dibuka. Menemukan %d Celah State/Sistemik yang mengerikan:\n", len(pandoraFindings))
	for _, f := range pandoraFindings {
		fmt.Printf("☣️  [%s] %s:%d\n   -> %s\n", f.VulnType, f.File, f.Line, f.Message)
	}

	outFile := filepath.Join(rootDir, "state", "scanner-reports", "pandoras_box_audit.md")
	writePandoraReport(outFile)
	fmt.Println("\n💀 Serpihan racun arsitekturnya udah disapu bersih ke:", outFile)
}

func scanPandora(filePath string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.AllErrors)
	if err != nil {
		return
	}

	// 1. Deteksi Global Mutable State Poisoning
	// Cari Deklarasi var global di level package (selain const) yang berpotensi menjadi ajang tabrakan antar agen AI.
	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if valSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range valSpec.Names {
						// Hindari mendeteksi error global idiomatis Go seperti var ErrNotFound = errors.New()
						if !strings.HasPrefix(name.Name, "Err") && !strings.HasPrefix(strings.ToLower(name.Name), "err") {
							msg := fmt.Sprintf("VARIABEL GLOBAL TERDETEKSI ('%s'). Jika FloworkOS menangani banyak agen AI/Request HTTP barengan, memodifikasi variabel ini bakal bikin 'State Poisoning'. State Agen A bisa kecampur State Agen B!", name.Name)
							recordPandora(fset, name.Pos(), filePath, "Global State Poisoning", msg)
						}
					}
				}
			}
		}
	}

	// Traversing the AST for function internals
	ast.Inspect(node, func(n ast.Node) bool {

		// 2. TOCTOU (Time of Check to Time of Use) File Race Conditions
		// Kita mendeteksi apakah di dalam suatu blok fungsi ada pemanggilan os.Stat / os.IsNotExist
		if block, ok := n.(*ast.BlockStmt); ok {
			hasStat := false
			hasFileMutation := false

			for _, stmt := range block.List {
				// Cek if/stat
				ast.Inspect(stmt, func(cn ast.Node) bool {
					if call, isCall := cn.(*ast.CallExpr); isCall {
						if fun, isSel := call.Fun.(*ast.SelectorExpr); isSel {
							if id, isId := fun.X.(*ast.Ident); isId && id.Name == "os" {
								if fun.Sel.Name == "Stat" || fun.Sel.Name == "IsNotExist" {
									hasStat = true
								}
								if fun.Sel.Name == "OpenFile" || fun.Sel.Name == "Create" || fun.Sel.Name == "Remove" {
									hasFileMutation = true
								}
							}
							if id, isId := fun.X.(*ast.Ident); isId && id.Name == "ioutil" {
								if fun.Sel.Name == "WriteFile" {
									hasFileMutation = true
								}
							}
						}
					}
					return true
				})
			}

			if hasStat && hasFileMutation {
				msg := "Mendeteksi pola Race Condition maut TOCTOU (os.Stat lalu modifikasi file). Dalam jeda mikrosekon antara pengecekan if file exists dan Write/Remove, Proses/Agen AI lain bisa masuk dan menghapus filenya duluan. Efeknya OS Panic!"
				recordPandora(fset, block.Pos(), filePath, "TOCTOU File Race", msg)
			}
		}

		// 3. Reflect Panics (The Dark Arts of Go)
		if call, ok := n.(*ast.CallExpr); ok {
			if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := fun.X.(*ast.Ident); ok && id.Name == "reflect" {
					if fun.Sel.Name == "ValueOf" || fun.Sel.Name == "TypeOf" {
						msg := "Menggunakan paket 'reflect' (Ilmu Hitam Go). Jika data berasal dari halusinasi JSON AI dan dipaksa refleksinya tanpa batas tipe statis, program agenmu akan sering Panic / Crash tak terduga (Type Assert Panic)."
						recordPandora(fset, call.Pos(), filePath, "Unsafe Reflection", msg)
					}
				}
			}
		}

		return true
	})
}

func recordPandora(fset *token.FileSet, pos token.Pos, file, vulnType, msg string) {
	pandoraFindings = append(pandoraFindings, PandoraVuln{
		VulnType: vulnType,
		File:     file,
		Line:     fset.Position(pos).Line,
		Message:  msg,
	})
}

func writePandoraReport(outFile string) {
	out, _ := os.Create(outFile)
	defer out.Close()

	out.WriteString("# 📦 KOTAK PANDORA: GLOBAL STATES & TOCTOU\n\n")
	out.WriteString("Ini adalah laporan level *God-Tier*. Tidak banyak orang yang repot-repot nge-scan *Time-of-Check to Time-of-Use*, apalagi *Global State Poisoning* yang bisa memicu Halusinasi Silang Antar Agen AI.\n\n")

	for _, f := range pandoraFindings {
		out.WriteString(fmt.Sprintf("---\n### ☣️ [%s]\n", f.VulnType))
		out.WriteString(fmt.Sprintf("**File Terkontaminasi**: `%s:%d`\n", f.File, f.Line))
		out.WriteString(fmt.Sprintf("**Diagnosis**: %s\n\n", f.Message))
	}
}
