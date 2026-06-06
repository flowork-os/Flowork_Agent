package apps

import (
	"os/exec"
	"testing"
)

// TestPolyglotNotepadSharedState — BUKTI inti ROADMAP 4: core app LINTAS BAHASA (Python via
// runtime:process) + "satu state, dua pengemudi". Driver-1 (agent) set → Driver-2 (human) get
// melihat state yang SAMA. Tanpa server/auth/LLM → murah & definitif.
func TestPolyglotNotepadSharedState(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 tak ada")
	}
	m := NewManager("../../apps")
	if err := m.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	defer m.Shutdown()
	if _, ok := m.Get("notepad"); !ok {
		t.Fatal("app 'notepad' tak ter-load")
	}
	// driver 1 (agent) set
	if _, err := m.InvokeOp("notepad", "set", map[string]any{"text": "hello from test"}, "agent"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// driver 2 (human) get → state SAMA (state persist di proses python yang sama)
	res, err := m.InvokeOp("notepad", "get", nil, "human-gui")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	mm, _ := res.(map[string]any)
	if mm["text"] != "hello from test" {
		t.Fatalf("shared-state GAGAL: %+v", res)
	}
	// append → bertambah
	if _, err := m.InvokeOp("notepad", "append", map[string]any{"text": "line2"}, "agent"); err != nil {
		t.Fatalf("append: %v", err)
	}
	res2, _ := m.InvokeOp("notepad", "get", nil, "human-gui")
	if mm2, _ := res2.(map[string]any); mm2["text"] != "hello from test\nline2" {
		t.Fatalf("append GAGAL: %+v", res2)
	}
	// op tak terdaftar DITOLAK
	if _, e := m.InvokeOp("notepad", "danger", nil, "agent"); e == nil {
		t.Fatal("op tak terdaftar harus ditolak")
	}
	// op→tool terdaftar (sisi agent)
	if m.regs["notepad"] == nil || len(m.regs["notepad"]) == 0 {
		t.Fatal("operasi tak terdaftar sebagai tool agent")
	}
}
