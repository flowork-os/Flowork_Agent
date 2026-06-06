package loket

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestFsScopedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sp := &syscallProviders{deps: Deps{ModuleDir: func(string) (string, error) { return dir, nil }}}
	if _, err := sp.fsWrite(context.Background(), "m", json.RawMessage(`{"path":"sub/a.txt","content":"hello"}`)); err != nil {
		t.Fatalf("fsWrite: %v", err)
	}
	r, err := sp.fsRead(context.Background(), "m", json.RawMessage(`{"path":"sub/a.txt"}`))
	if err != nil {
		t.Fatalf("fsRead: %v", err)
	}
	var got struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(r, &got)
	if got.Content != "hello" {
		t.Errorf("roundtrip wrong: %q", got.Content)
	}
	if _, err := sp.fsList(context.Background(), "m", json.RawMessage(`{"path":"sub"}`)); err != nil {
		t.Errorf("fsList: %v", err)
	}
}

// TestFsEscapeRejected — the kernel refuses any path that escapes the module's own
// folder, so fs.* can never touch another module's or the kernel's files.
func TestFsEscapeRejected(t *testing.T) {
	dir := t.TempDir()
	sp := &syscallProviders{deps: Deps{ModuleDir: func(string) (string, error) { return dir, nil }}}
	for _, p := range []string{"../escape.txt", "../../etc/passwd", "sub/../../x"} {
		if _, err := sp.fsRead(context.Background(), "m", json.RawMessage(`{"path":"`+p+`"}`)); err == nil {
			t.Errorf("path %q should be rejected (escapes module folder)", p)
		}
	}
}

func TestExecRunBounded(t *testing.T) {
	dir := t.TempDir()
	sp := &syscallProviders{deps: Deps{ModuleDir: func(string) (string, error) { return dir, nil }}}
	r, err := sp.execRun(context.Background(), "m", json.RawMessage(`{"cmd":"echo","args":["halo-loket"]}`))
	if err != nil {
		t.Fatalf("execRun: %v", err)
	}
	var got struct {
		Stdout string `json:"stdout"`
		Code   int    `json:"code"`
	}
	_ = json.Unmarshal(r, &got)
	if got.Code != 0 || !strings.Contains(got.Stdout, "halo-loket") {
		t.Errorf("exec wrong: code=%d out=%q", got.Code, got.Stdout)
	}
}

func TestRegistryDiscovery(t *testing.T) {
	rp := &registryProviders{deps: Deps{Modules: func() []ModuleInfo {
		return []ModuleInfo{
			{ID: "a", Kind: "agent"},
			{ID: "svc", Kind: "service", Provides: []string{"desktop.click"}},
		}
	}}}
	r, _ := rp.list(context.Background(), "x", json.RawMessage(`{"kind":"service"}`))
	var l struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(r, &l)
	if l.Count != 1 {
		t.Errorf("list kind=service want 1 got %d", l.Count)
	}
	r2, _ := rp.providers(context.Background(), "x", json.RawMessage(`{"cap":"desktop.click"}`))
	var p struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(r2, &p)
	if p.Count != 1 {
		t.Errorf("providers of desktop.click want 1 got %d", p.Count)
	}
}
