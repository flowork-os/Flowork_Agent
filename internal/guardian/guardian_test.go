package guardian

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestArmVerifyTamper(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // vault → temp home

	// file inti palsu buat dijaga.
	core := filepath.Join(t.TempDir(), "kernel_core.go")
	if err := os.WriteFile(core, []byte("package x // v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	// arm: rekam baseline (binary test @self + core).
	v, err := Arm([]string{core}, "2026-06-07T00:00:00Z")
	if err != nil {
		t.Fatalf("arm: %v", err)
	}
	if !v.Armed {
		t.Fatal("harus armed")
	}
	if _, ok := v.Baseline[selfKey]; !ok {
		t.Fatal("baseline harus punya @self (binary)")
	}

	// belum diubah → verify cocok.
	if ok, probs := v.Verify(); !ok {
		t.Fatalf("verify harus OK, dapat: %v", probs)
	}

	// TAMPER file inti → verify gagal.
	if err := os.WriteFile(core, []byte("package x // HACKED"), 0o644); err != nil {
		t.Fatal(err)
	}
	v2, _ := Load()
	if ok, probs := v2.Verify(); ok || len(probs) == 0 {
		t.Fatal("verify harus GAGAL setelah tamper")
	}

	// file inti DIHAPUS → verify gagal (hilang).
	os.Remove(core)
	if ok, _ := v2.Verify(); ok {
		t.Fatal("verify harus gagal kalau file inti hilang")
	}

	// disarm → armed=false.
	if err := Disarm(); err != nil {
		t.Fatal(err)
	}
	if v3, _ := Load(); v3.Armed {
		t.Fatal("harus disarmed")
	}
}

func TestDangerousPath(t *testing.T) {
	danger := []string{"/api/agents/tools/run", "/api/plugins/install", "/api/coder/generate", "/api/scanner/run", "/api/kernel/call"}
	for _, p := range danger {
		if !IsDangerousPath(p) {
			t.Errorf("%s harusnya dangerous", p)
		}
	}
	safe := []string{"/api/guardian/status", "/api/system/health", "/api/agents", "/login", "/api/chat"}
	for _, p := range safe {
		if IsDangerousPath(p) {
			t.Errorf("%s harusnya AMAN (jangan diblok)", p)
		}
	}
}

func TestSafeModeMiddleware(t *testing.T) {
	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true; w.WriteHeader(200) })
	mw := SafeModeMiddleware(next)

	call := func(path string) int {
		hit = false
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		return rec.Code
	}

	// sebelum safe-mode: semua lewat.
	if code := call("/api/agents/tools/run"); code != 200 || !hit {
		t.Fatalf("pra-safe-mode: exec harus lewat, dapat %d", code)
	}

	// aktifkan safe-mode.
	EnterSafeMode()
	if !SafeMode() {
		t.Fatal("SafeMode harus true")
	}
	if code := call("/api/agents/tools/run"); code != http.StatusServiceUnavailable || hit {
		t.Fatalf("safe-mode: exec harus 503 (blok), dapat %d hit=%v", code, hit)
	}
	if code := call("/api/agents"); code != 200 || !hit {
		t.Fatalf("safe-mode: baca harus tetap lewat, dapat %d", code)
	}
}
