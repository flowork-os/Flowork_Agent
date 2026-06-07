package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"testing"
)

// TestKernelFreeze — ENFORCEMENT lapis-1 Kernel FREEZE. Recompute SHA256 tiap file
// inti dan bandingkan dgn manifest KERNEL_FREEZE.md. Kalau file beku berubah tanpa
// update manifest → test GAGAL. Ini bikin "ditulis sekali, ga diedit lagi" terjaga:
// mengubah kernel inti = keputusan SADAR (unfreeze → edit → regenerate manifest),
// bukan kecelakaan. Lapis-2 (OS-immutable / runtime ARM) terpisah.
//
// Regenerate manifest setelah unfreeze yang disengaja owner:
//   while read f; do sha256sum "$f"; done < <(grep -oE 'internal/[^ ]+\.go' KERNEL_FREEZE.md)
var freezeLineRe = regexp.MustCompile(`^([0-9a-f]{64})\s+(\S+\.go)$`)

func TestKernelFreeze(t *testing.T) {
	mf, err := os.Open("KERNEL_FREEZE.md")
	if err != nil {
		t.Fatalf("KERNEL_FREEZE.md tak terbaca: %v", err)
	}
	defer mf.Close()

	checked := 0
	sc := bufio.NewScanner(mf)
	for sc.Scan() {
		m := freezeLineRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		want, path := m[1], m[2]
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Errorf("file beku hilang/tak terbaca: %s (%v)", path, rerr)
			continue
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("FILE INTI BERUBAH (freeze dilanggar): %s\n  manifest=%s\n  aktual  =%s\n  → kalau ini disengaja: unfreeze sadar + regenerate KERNEL_FREEZE.md", path, want, got)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("manifest KERNEL_FREEZE.md tak punya entri checksum — freeze tak ter-enforce")
	}
	t.Logf("freeze OK: %d file inti cocok manifest", checked)
}
