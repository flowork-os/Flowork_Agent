// selfevolve_coreapply_test.go — bukti deterministik guard core-apply (B1). Batas keamanan
// 🔴: path-traversal ditolak, file LOCKED kedeteksi, fence ke-strip, modul Go ketemu.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvolveSafeRepoPath(t *testing.T) {
	root := "/repo"
	cases := []struct {
		rel     string
		wantOK  bool
		wantRel string
	}{
		{"agent/foo.go", true, "agent/foo.go"},
		{"internal/x/y.go", true, "internal/x/y.go"},
		{"../etc/passwd", false, ""},
		{"/etc/passwd", false, ""},
		{"", false, ""},
		{"a/../../b", false, ""},
		{"./a.go", true, "a.go"},
	}
	for _, c := range cases {
		got, ok := evolveSafeRepoPath(root, c.rel)
		if ok != c.wantOK {
			t.Errorf("rel=%q: ok=%v want %v", c.rel, ok, c.wantOK)
			continue
		}
		if ok && got != c.wantRel {
			t.Errorf("rel=%q: got %q want %q", c.rel, got, c.wantRel)
		}
	}
}

func TestEvolveStripFence(t *testing.T) {
	cases := map[string]string{
		"```go\npackage x\n```": "package x",
		"plain code":            "plain code",
		"```\nhello\n```":       "hello",
		"  ```js\na=1\n```  ":   "a=1",
	}
	for in, want := range cases {
		if got := evolveStripFence(in); got != want {
			t.Errorf("strip(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEvolveFileLocked(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.go")
	_ = os.WriteFile(locked, []byte("// === LOCKED FILE (soft) ===\npackage x\n"), 0o644)
	plain := filepath.Join(dir, "plain.go")
	_ = os.WriteFile(plain, []byte("package x\n"), 0o644)
	if !evolveFileLocked(locked) {
		t.Error("locked file not detected")
	}
	if evolveFileLocked(plain) {
		t.Error("plain file falsely flagged locked")
	}
	if evolveFileLocked(filepath.Join(dir, "nope.go")) {
		t.Error("missing file should not be locked")
	}
}

func TestEvolveResolveTarget(t *testing.T) {
	root := t.TempDir()
	// root/agent/go.mod + root/agent/internal/agentdb (modul agent, package dir ada).
	_ = os.MkdirAll(filepath.Join(root, "agent", "internal", "agentdb"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "agent", "go.mod"), []byte("module x\n"), 0o644)
	// proposal target relatif modul agent → harus di-prefix "agent/".
	if got := evolveResolveTarget(root, "internal/agentdb/new.go"); got != filepath.Join("agent", "internal/agentdb/new.go") {
		t.Errorf("resolve agent-relative: got %q", got)
	}
	// udah repo-relatif (folder induk ada langsung) → as-is.
	if got := evolveResolveTarget(root, "agent/internal/agentdb/new.go"); got != "agent/internal/agentdb/new.go" {
		t.Errorf("resolve already-correct: got %q", got)
	}
	// folder bener-bener baru (ga ada di mana-mana) → as-is.
	if got := evolveResolveTarget(root, "brandnew/pkg/x.go"); got != "brandnew/pkg/x.go" {
		t.Errorf("resolve brand-new: got %q", got)
	}
}

func TestEvolveModuleDir(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "agent")
	_ = os.MkdirAll(filepath.Join(modDir, "internal", "x"), 0o755)
	_ = os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module x\n"), 0o644)
	got := evolveModuleDir(root, "agent/internal/x/new.go")
	if got != modDir {
		t.Errorf("module dir=%q want %q", got, modDir)
	}
	// di luar modul (no go.mod) → "".
	if d := evolveModuleDir(root, "docs/readme.md"); d != "" {
		t.Errorf("no-module path should give empty, got %q", d)
	}
}
