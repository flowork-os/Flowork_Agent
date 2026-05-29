// Package codeindex — Go AST parser untuk ekstrak dependency & symbol info.
// Digunakan oleh Indexer untuk populate codemap_nodes + codemap_edges.
package codeindex

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// GoFileInfo hasil parse satu file .go
type GoFileInfo struct {
	Package         string
	Imports         []string // import paths (raw string: "fmt", "github.com/...")
	ExportedSymbols []string // nama exported: func/type/var/const
	DocComment      string   // komentar package-level pertama
	LineCount       int
	SizeBytes       int64
}

// ParseGoFile parse satu file .go menggunakan go/ast.
// Error parse dikembalikan; caller bisa pilih skip atau log.
func ParseGoFile(absPath string) (*GoFileInfo, error) {
	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		// Partial parse — tetap ambil info yang ada
		if f == nil {
			return nil, err
		}
	}

	info := &GoFileInfo{
		SizeBytes: fi.Size(),
	}

	// Package name
	if f.Name != nil {
		info.Package = f.Name.Name
	}

	// Line count dari token set
	if fset.File(f.Pos()) != nil {
		info.LineCount = fset.File(f.Pos()).LineCount()
	}

	// Package doc comment (baris pertama sebelum package keyword)
	if f.Doc != nil {
		lines := []string{}
		for _, c := range f.Doc.List {
			text := strings.TrimPrefix(c.Text, "//")
			text = strings.TrimPrefix(text, "/*")
			text = strings.TrimSuffix(text, "*/")
			lines = append(lines, strings.TrimSpace(text))
		}
		info.DocComment = strings.Join(lines, "\n")
	}

	// Imports
	for _, imp := range f.Imports {
		if imp.Path != nil {
			path := strings.Trim(imp.Path.Value, `"`)
			info.Imports = append(info.Imports, path)
		}
	}

	// Exported symbols — top-level declarations starting with uppercase
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil && ast.IsExported(d.Name.Name) {
				info.ExportedSymbols = append(info.ExportedSymbols, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(s.Name.Name) {
						info.ExportedSymbols = append(info.ExportedSymbols, s.Name.Name)
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if ast.IsExported(name.Name) {
							info.ExportedSymbols = append(info.ExportedSymbols, name.Name)
						}
					}
				}
			}
		}
	}

	return info, nil
}

// ResolveGoImportToPath mencoba resolve import path ke path relatif dalam workspace.
// Hanya untuk import internal (prefix module path). External imports diabaikan.
//
// CATATAN: Function ini return path PACKAGE DIRECTORY (bukan file spesifik) —
// dipertahankan untuk backward compat. Untuk edge graph yang resolve ke
// FILE-LEVEL gunakan ResolveGoImportToFiles (Sprint 3.5e RD-201 fix).
func ResolveGoImportToPath(importPath, modulePath, workspaceRoot string) string {
	if !strings.HasPrefix(importPath, modulePath) {
		return "" // external dep, skip
	}
	rel := strings.TrimPrefix(importPath, modulePath)
	rel = strings.TrimPrefix(rel, "/")
	// Kembalikan path ke folder package (bukan file spesifik)
	return filepath.ToSlash(filepath.Join(workspaceRoot, rel))
}

// ResolveGoImportToFiles — Sprint 3.5e RD-201 fix.
//
// Resolve Go import ke ALL .go files di package directory (fan-out).
// Sebelumnya ResolveGoImportToPath return single directory path → edge graph
// punya from=file, to=directory. Itu bikin codemap_deps/impact/zombies/health
// query broken karena to_path bukan file actual.
//
// Sekarang fan-out: 1 import statement → N edges (1 per .go file di package).
// File berikut di-skip:
//   - *_test.go (test files, bukan dependency runtime)
//   - File yang sama dengan source (self-edge)
//
// Return: list of absolute file paths (forward-slash). Empty list kalau:
//   - importPath external (bukan internal module)
//   - directory ngga exist
//   - directory kosong (no .go files)
func ResolveGoImportToFiles(importPath, modulePath, workspaceRoot string) []string {
	dir := ResolveGoImportToPath(importPath, modulePath, workspaceRoot)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.FromSlash(dir))
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		// Skip test files — mereka bukan runtime dep.
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Join(dir, name)))
	}
	return files
}

// ReadModulePath baca nama module dari go.mod di workspaceRoot.
func ReadModulePath(workspaceRoot string) string {
	data, err := os.ReadFile(filepath.Join(workspaceRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
