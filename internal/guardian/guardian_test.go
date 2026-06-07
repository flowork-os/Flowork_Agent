package guardian

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSealer — sealer in-memory buat test (hindari chattr asli yang butuh root).
type fakeSealer struct {
	sealed map[string]bool
	failOn string
}

func newFake() *fakeSealer { return &fakeSealer{sealed: map[string]bool{}} }
func (f *fakeSealer) Name() string { return "fake" }
func (f *fakeSealer) Seal(p string) error {
	if p == f.failOn {
		return errFakeSeal
	}
	f.sealed[p] = true
	return nil
}
func (f *fakeSealer) Unseal(p string) error          { delete(f.sealed, p); return nil }
func (f *fakeSealer) IsSealed(p string) (bool, error) { return f.sealed[p], nil }

var errFakeSeal = &fakeErr{"seal denied"}

type fakeErr struct{ s string }

func (e *fakeErr) Error() string { return e.s }

func TestArmVerifyTamper(t *testing.T) {
	defer setSealerForTest(newFake())() // jangan sentuh chattr asli
	t.Setenv("HOME", t.TempDir())       // vault → temp home

	// file inti palsu buat dijaga.
	core := filepath.Join(t.TempDir(), "kernel_core.go")
	if err := os.WriteFile(core, []byte("package x // v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	// arm: rekam baseline (binary test @self + core).
	v, err := Arm([]string{core}, "2026-06-07T00:00:00Z", true)
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

func TestSealOrchestration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// sukses: arm → Sealed=true, vault + binary ke-seal; disarm → ke-unseal.
	fake := newFake()
	restore := setSealerForTest(fake)
	v, err := Arm(nil, "t0", true)
	if err != nil {
		t.Fatalf("arm: %v", err)
	}
	if !v.Sealed {
		t.Fatal("harus Sealed=true dgn sealer sukses")
	}
	if !fake.sealed[VaultPath()] {
		t.Fatal("vault harus ke-seal (terakhir)")
	}
	if err := Disarm(); err != nil {
		t.Fatalf("disarm: %v", err)
	}
	if len(fake.sealed) != 0 {
		t.Fatalf("semua harus ter-unseal setelah disarm, sisa: %v", fake.sealed)
	}
	restore()

	// DEGRADE: sealer gagal (no-root) → Sealed=false TAPI armed tetap true (detection-only),
	// dan ga ada yang nyangkut ke-seal (rollback).
	fail := newFake()
	if ex, e := os.Executable(); e == nil {
		if r, e2 := filepath.EvalSymlinks(ex); e2 == nil {
			fail.failOn = r
		}
	}
	defer setSealerForTest(fail)()
	v2, err := Arm(nil, "t1", true)
	if err != nil {
		t.Fatalf("arm degrade: %v", err)
	}
	if v2.Sealed {
		t.Fatal("harus Sealed=false saat seal gagal")
	}
	if !v2.Armed {
		t.Fatal("arm harus tetap sukses (detection-only) walau seal gagal")
	}
	if len(fail.sealed) != 0 {
		t.Fatalf("rollback: ga boleh ada sisa ke-seal, dapat: %v", fail.sealed)
	}
}

func TestArmDetectionOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := newFake()
	defer setSealerForTest(fake)() // sealer JALAN, tapi attemptSeal=false → ga boleh dipanggil
	v, err := Arm(nil, "t0", false)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Armed {
		t.Fatal("auto/detection harus tetap armed")
	}
	if v.Sealed {
		t.Fatal("detection-only ga boleh Sealed")
	}
	if len(fake.sealed) != 0 {
		t.Fatalf("attemptSeal=false → Seal ga boleh kepanggil, dapat: %v", fake.sealed)
	}
}

func anyContains(items []string, sub string) bool {
	for _, s := range items {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestSentinelCapDrift(t *testing.T) {
	safeMode.Store(false)
	t.Setenv("HOME", t.TempDir())
	defer setSealerForTest(newFake())()
	core := filepath.Join(t.TempDir(), "k.go")
	if err := os.WriteFile(core, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Arm([]string{core}, "t0", true); err != nil {
		t.Fatal(err)
	}

	var alerts []string
	al := func(m string) { alerts = append(alerts, m) }
	baseline := map[string]bool{} // start kosong → exec agent1 = BARU

	caps := func() map[string][]string { return map[string][]string{"agent1": {"exec:power"}} }
	sentinelTick(caps, al, baseline)
	if !anyContains(alerts, "agent1 → exec:power") {
		t.Fatalf("harus alert cap berbahaya baru, dapat: %v", alerts)
	}
	n := len(alerts)
	sentinelTick(caps, al, baseline) // cap sama → ga boleh re-alert
	if len(alerts) != n {
		t.Fatal("cap yang sama ga boleh re-alert tiap tick")
	}
	caps2 := func() map[string][]string { return map[string][]string{"agent1": {"exec:power", "secret:read"}} }
	sentinelTick(caps2, al, baseline)
	if !anyContains(alerts, "secret:read") {
		t.Fatal("cap berbahaya baru kedua harus alert")
	}
}

func TestSentinelIntegrityRuntime(t *testing.T) {
	safeMode.Store(false)
	t.Setenv("HOME", t.TempDir())
	defer setSealerForTest(newFake())()
	core := filepath.Join(t.TempDir(), "k.go")
	os.WriteFile(core, []byte("v1"), 0o644)
	if _, err := Arm([]string{core}, "t0", true); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(core, []byte("HACKED-at-runtime"), 0o644) // tamper PASCA-arm

	var alerts []string
	sentinelTick(nil, func(m string) { alerts = append(alerts, m) }, map[string]bool{})
	if !SafeMode() {
		t.Fatal("integritas runtime gagal → harus masuk SAFE-MODE")
	}
	if !anyContains(alerts, "integritas") {
		t.Fatalf("harus alert integritas, dapat: %v", alerts)
	}
	safeMode.Store(false) // bersihin buat test lain
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
