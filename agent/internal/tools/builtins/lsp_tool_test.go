package builtins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowork-gui/internal/tools"
)

func setupLSPModule(t *testing.T) (context.Context, string) {
	t.Helper()
	dir := t.TempDir()
	must := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("go.mod", "module lsptest\n\ngo 1.21\n")
	must("main.go", "package main\n\n// Greet bikin sapaan.\nfunc Greet(name string) string {\n\treturn \"hi \" + name\n}\n\nfunc main() {\n\t_ = Greet(\"x\")\n}\n")
	return tools.WithSharedDir(context.Background(), dir), dir
}

func lspCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		lspMu.Lock()
		for root, c := range lspClients {
			c.shutdown()
			delete(lspClients, root)
		}
		lspMu.Unlock()
	})
}

func TestLSPHoverAndDefinition(t *testing.T) {
	if goplsPath() == "" {
		t.Skip("gopls ga keinstall — skip test LSP")
	}
	ctx, _ := setupLSPModule(t)
	lspCleanup(t)

	// Hover di deklarasi Greet (occurrence 1 = token IDENT pertama, skip komentar) → info tipe.
	hres, err := (lspTool{}).Run(ctx, map[string]any{
		"file_path": "main.go", "symbol": "Greet", "operation": "hover",
	})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	hover, _ := hres.Output.(map[string]any)["hover"].(string)
	if !strings.Contains(hover, "Greet") {
		t.Errorf("hover ga ada info Greet: %q", hover)
	}

	// Definition dari PEMAKAIAN Greet (occurrence 2) → nunjuk ke deklarasi (main.go).
	dres, err := (lspTool{}).Run(ctx, map[string]any{
		"file_path": "main.go", "symbol": "Greet", "operation": "definition", "occurrence": float64(2),
	})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	results, _ := dres.Output.(map[string]any)["results"].([]map[string]any)
	if len(results) == 0 {
		t.Fatalf("definition kosong (gopls belum index?), output: %+v", dres.Output)
	}
	if f, _ := results[0]["file"].(string); !strings.HasSuffix(f, "main.go") {
		t.Errorf("definition harusnya di main.go, dapet %v", f)
	}
}

func TestLSPReferences(t *testing.T) {
	if goplsPath() == "" {
		t.Skip("gopls ga keinstall — skip test LSP")
	}
	ctx, _ := setupLSPModule(t)
	lspCleanup(t)

	rres, err := (lspTool{}).Run(ctx, map[string]any{
		"file_path": "main.go", "symbol": "Greet", "operation": "references",
	})
	if err != nil {
		t.Fatalf("references: %v", err)
	}
	cnt, _ := rres.Output.(map[string]any)["count"].(int)
	if cnt < 2 { // deklarasi + minimal 1 pemakaian
		t.Errorf("references harusnya >=2 (decl+usage), dapet %d", cnt)
	}
}

func TestLSPRejectsNonGoAndEscape(t *testing.T) {
	ctx, dir := setupLSPModule(t)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("x"), 0o644)

	if _, err := (lspTool{}).Run(ctx, map[string]any{
		"file_path": "note.txt", "symbol": "x", "operation": "hover",
	}); err == nil {
		t.Error("harus nolak non-.go")
	}
	if _, err := (lspTool{}).Run(ctx, map[string]any{
		"file_path": "../escape.go", "symbol": "x",
	}); err == nil {
		t.Error("harus nolak file_path '..' (workspace escape)")
	}
	if _, err := (lspTool{}).Run(ctx, map[string]any{
		"file_path": "main.go", "symbol": "TidakAda123",
	}); err == nil {
		t.Error("harus error kalau symbol ga ketemu di file")
	}
}

func TestLSPFindSymbolPos(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.go")
	os.WriteFile(p, []byte("package x\nvar Foo = 1\nvar FooBar = Foo + 2\n"), 0o644)
	// occurrence 1 = 'Foo' di baris 2 (bukan substring 'FooBar').
	line, col, snip, err := findSymbolPos(p, "Foo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if line != 1 || col != 4 {
		t.Errorf("pos Foo occ1 = (%d,%d), mau (1,4); snip=%q", line, col, snip)
	}
	// occurrence 2 = 'Foo' di baris 3 (pemakaian), bukan 'FooBar'.
	line2, _, _, err := findSymbolPos(p, "Foo", 2)
	if err != nil || line2 != 2 {
		t.Errorf("pos Foo occ2 line = %d (err %v), mau 2", line2, err)
	}
}
