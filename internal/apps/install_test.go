package apps

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"flowork-gui/internal/tools"
)

// buildAppPack — bikin .fwpack app minimal di memori (Python core echo state).
func buildAppPack(t *testing.T, id string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("plugin.json", `{"kind":"app","id":"`+id+`"}`)
	write("apps/"+id+`/manifest.json`, `{
		"id":"`+id+`","kind":"app","name":"Test App","runtime":"process",
		"core_entry":"python3 core.py","gui_entry":"ui/index.html",
		"operations":[
			{"name":"get","tool":true},
			{"name":"set","tool":true,"mutates":true}
		]}`)
	// core: simpan text di memori, get/set — bukti satu state.
	write("apps/"+id+`/core.py`, `import sys,json
state={"text":""}
v=0
for line in sys.stdin:
    line=line.strip()
    if not line: continue
    r=json.loads(line); op=r.get("op"); a=r.get("args") or {}
    if op=="set":
        state["text"]=str(a.get("text","")); v+=1
        out={"result":{"ok":True},"state_version":v}
    elif op=="get":
        out={"result":dict(state),"state_version":v}
    else:
        out={"error":"unknown"}
    sys.stdout.write(json.dumps(out)+"\n"); sys.stdout.flush()
`)
	write("apps/"+id+`/ui/index.html`, `<!doctype html><title>t</title>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAppInstallUninstallLifecycle(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	SetDefault(m)
	const id = "testapp"
	raw := buildAppPack(t, id)

	// 1) tanpa consent → 403 consent_required (core = exec OS).
	if body, status := m.installFromZip(raw, false); status != 403 || body["consent_required"] != true {
		t.Fatalf("expected 403 consent_required, got status=%d body=%v", status, body)
	}

	// 2) dengan consent → sukses (status 0), app live.
	if body, status := m.installFromZip(raw, true); status != 0 || body["ok"] != true {
		t.Fatalf("install gagal: status=%d body=%v", status, body)
	}
	if _, ok := m.Get(id); !ok {
		t.Fatal("app harusnya terdaftar setelah install")
	}
	// tool harus teregister (app_testapp_get & _set).
	if _, ok := tools.Lookup(toolName(id, "get")); !ok {
		t.Fatalf("tool %s harusnya teregister", toolName(id, "get"))
	}

	// 3) satu state, dua pengemudi: set (agent) → get (human) lihat nilai sama.
	if _, err := m.InvokeOp(id, "set", map[string]any{"text": "halo"}, "agent"); err != nil {
		t.Fatalf("set: %v", err)
	}
	out, err := m.InvokeOp(id, "get", nil, "human-gui")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if mp, _ := out.(map[string]any); mp["text"] != "halo" {
		t.Fatalf("state ga kebagi: %v", out)
	}

	// 4) uninstall → app hilang, folder hilang, tool ke-unregister.
	if err := m.Uninstall(id); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, ok := m.Get(id); ok {
		t.Fatal("app harusnya hilang setelah uninstall")
	}
	if _, ok := tools.Lookup(toolName(id, "get")); ok {
		t.Fatal("tool harusnya ke-unregister setelah uninstall")
	}
	if _, err := os.Stat(filepath.Join(dir, id)); !os.IsNotExist(err) {
		t.Fatal("folder app harusnya terhapus")
	}
}
