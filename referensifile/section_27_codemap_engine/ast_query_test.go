package factmemory

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildCallGraph_detectsBasicCalls(t *testing.T) {
	ws := t.TempDir()

	writeGoFile(t, ws, "main.go", `package m

func Alpha() int { return Beta() + 1 }

func Beta() int { return 2 }

type Svc struct{}

func (s *Svc) DoWork() { Alpha() }
`)

	graph, err := BuildCallGraph(ws)
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}

	// Expect: Alpha calls Beta; DoWork calls Alpha.
	callers := graph.FindCallers("Beta")
	if len(callers) == 0 {
		t.Fatalf("expected at least 1 caller of Beta, got 0")
	}
	if callers[0].Callee != "Beta" {
		t.Errorf("callee = %q, want Beta", callers[0].Callee)
	}
	if callers[0].Caller != "m.Alpha" {
		t.Errorf("caller = %q, want m.Alpha", callers[0].Caller)
	}

	alphaCallers := graph.FindCallers("Alpha")
	if len(alphaCallers) == 0 {
		t.Error("expected Alpha to have at least 1 caller (DoWork)")
	}
	found := false
	for _, c := range alphaCallers {
		if c.Caller == "m.Svc.DoWork" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Svc.DoWork → Alpha call-site not detected; callers: %+v", alphaCallers)
	}
}

func TestLoadCallGraph_persistRoundtrip(t *testing.T) {
	ws := t.TempDir()
	writeGoFile(t, ws, "a.go", `package a

func One() {}

func Two() { One() }
`)

	_, err := BuildCallGraph(ws)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCallGraph(ws)
	if err != nil {
		t.Fatalf("LoadCallGraph: %v", err)
	}
	if len(loaded.Calls) == 0 {
		t.Error("loaded graph empty; expected ≥1 call entry")
	}
	callers := loaded.FindCallers("One")
	if len(callers) == 0 {
		t.Error("expected One to have callers after persist roundtrip")
	}
}

func TestFindCallers_suffixMatch(t *testing.T) {
	g := &CallGraph{
		Calls: []CallSite{
			{Caller: "a.Foo", Callee: "pkg.Open", Package: "a", Filepath: "a.go", Line: 3},
			{Caller: "b.Bar", Callee: "Open", Package: "b", Filepath: "b.go", Line: 5},
		},
	}
	results := g.FindCallers("Open")
	if len(results) != 2 {
		t.Errorf("expected 2 matches (exact + suffix), got %d", len(results))
	}
}

func TestFindDecl_exactAndMethodMatch(t *testing.T) {
	idx := &ASTIndex{
		Nodes: []ASTNode{
			{Type: "func", Name: "Open", Package: "a", Filepath: "a.go", Line: 1},
			{Type: "func", Name: "(Pool) Open", Package: "p", Filepath: "pool.go", Line: 5},
			{Type: "func", Name: "Close", Package: "x", Filepath: "x.go", Line: 10},
		},
	}
	results := idx.FindDecl("Open")
	if len(results) != 2 {
		t.Errorf("expected 2 Open matches, got %d: %+v", len(results), results)
	}
}

func TestBuildCallGraph_skipsTestFiles(t *testing.T) {
	ws := t.TempDir()
	writeGoFile(t, ws, "main.go", `package m

func Real() {}
`)
	writeGoFile(t, ws, "main_test.go", `package m

import "testing"

func TestReal(t *testing.T) { Real() }
`)

	graph, err := BuildCallGraph(ws)
	if err != nil {
		t.Fatal(err)
	}
	// Test file calls should NOT appear — walker skips _test.go
	if callers := graph.FindCallers("Real"); len(callers) != 0 {
		t.Errorf("expected test-file calls to be skipped; got %+v", callers)
	}
}
