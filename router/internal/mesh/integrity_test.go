package mesh

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resetIntegrity() {
	integrityOnce = sync.Once{}
	coreClean, coreRoot, coreCheckedCnt = true, "", 0
}

// TestCoreIntegrityCleanAndTamper buktiin: manifest cocok → CoreClean()=true;
// 1 hash di manifest beda → tampered (CoreClean()=false). Pakai ../router/ (real
// dir relatif dari cwd internal/mesh) biar os.ReadFile resolve beneran.
func TestCoreIntegrityCleanAndTamper(t *testing.T) {
	real := "../router/dispatcher.go"
	data, err := os.ReadFile(real)
	if err != nil {
		t.Skipf("file uji %s tak ada: %v", real, err)
	}
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])
	dir := t.TempDir()

	mfClean := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(mfClean, []byte(good+"  ../router/dispatcher.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWORK_KERNEL_MANIFEST", mfClean)
	resetIntegrity()
	if !CoreClean() {
		t.Fatal("manifest cocok HARUS clean=true")
	}
	if CoreCheckedCount() != 1 {
		t.Fatalf("checked=%d, mau 1", CoreCheckedCount())
	}
	if CoreRootHash() == "" {
		t.Fatal("root-hash kosong padahal verified")
	}

	mfBad := filepath.Join(dir, "bad.md")
	bad := "0000000000000000000000000000000000000000000000000000000000000000"
	if err := os.WriteFile(mfBad, []byte(bad+"  ../router/dispatcher.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWORK_KERNEL_MANIFEST", mfBad)
	resetIntegrity()
	if CoreClean() {
		t.Fatal("hash manifest BEDA harus tampered (clean=false) — gate gagal deteksi")
	}

	// Manifest tak ada → tak bisa verifikasi → jangan blokir (clean=true).
	t.Setenv("FLOWORK_KERNEL_MANIFEST", filepath.Join(dir, "nope.md"))
	resetIntegrity()
	if !CoreClean() {
		t.Fatal("manifest absen HARUS clean=true (degrade aman)")
	}
}
