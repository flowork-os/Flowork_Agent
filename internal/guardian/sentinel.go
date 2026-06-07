// sentinel.go — ROADMAP Guardian FASE 3 (L4): pengawas RUNTIME.
//
// Boot-gate (FASE 1) cuma cek sekali saat start. Sentinel = goroutine periodik yang nangkep
// tamper SAAT JALAN, tiap tick (saat armed):
//   (A) re-verify integritas  — file inti/binary berubah pasca-boot (relevan di detection-only)
//   (B) seal-drift            — file yang harusnya immutable tapi udah ke-unseal (chattr -i?)
//   (C) cap-drift             — agent dapet capability BERBAHAYA baru tanpa lewat install (eskalasi)
// Anomali → alert owner (Telegram). Integritas gagal → +SAFE-MODE (sama dgn boot-gate, D1).
package guardian

import (
	"context"
	"sort"
	"strings"
	"time"
)

// CapSource — balik map agentID -> daftar capability BERBAHAYA yang dideklarasi. Di-inject dari
// main (yang tahu cara enumerate manifest agent) → guardian tetap decoupled.
type CapSource func() map[string][]string

// RunSentinel — jalanin pengawas runtime sampai ctx selesai. interval<=0 → default 5 menit.
// alert = notifyOwnerTelegram (atau apa pun). caps boleh nil (skip cek C).
func RunSentinel(ctx context.Context, interval time.Duration, caps CapSource, alert func(string)) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	baseline := map[string]bool{} // "agent|cap" yang sudah dikenal (di-snapshot saat start)
	if caps != nil {
		for a, cs := range caps() {
			for _, c := range cs {
				baseline[a+"|"+c] = true
			}
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sentinelTick(caps, alert, baseline)
		}
	}
}

// sentinelTick — satu ronde cek (dipisah biar testable). baseline di-mutate (agent|cap baru ditandai).
func sentinelTick(caps CapSource, alert func(string), baseline map[string]bool) {
	v, err := Load()
	if err != nil || !v.Armed {
		return // pasif kalau belum di-arm
	}
	if alert == nil {
		alert = func(string) {}
	}

	// (A) integritas runtime.
	if ok, probs := v.Verify(); !ok && !SafeMode() {
		EnterSafeMode()
		alert("🛡️ GUARDIAN sentinel: integritas RUNTIME GAGAL — SAFE-MODE aktif.\n• " + strings.Join(capN(probs, 12), "\n• "))
	}

	// (B) seal-drift (cuma kalau mode immutable).
	if v.Sealed {
		s := DefaultSealer()
		targets := append(immutableTargets(), VaultPath())
		for _, p := range targets {
			if ok, ierr := s.IsSealed(p); ierr == nil && !ok {
				alert("🛡️ GUARDIAN sentinel: SEAL DRIFT — '" + p + "' ga lagi immutable (ada yang membuka segel?).")
			}
		}
	}

	// (C) cap-drift: cap berbahaya BARU yang belum dikenal.
	if caps != nil {
		var fresh []string
		for a, cs := range caps() {
			for _, c := range cs {
				k := a + "|" + c
				if !baseline[k] {
					baseline[k] = true
					fresh = append(fresh, a+" → "+c)
				}
			}
		}
		if len(fresh) > 0 {
			sort.Strings(fresh)
			alert("🛡️ GUARDIAN sentinel: capability BERBAHAYA baru terdeteksi (konfirmasi kalau ini emang lo):\n• " + strings.Join(capN(fresh, 20), "\n• "))
		}
	}
}

// capN — potong slice biar alert ga kepanjangan.
func capN(items []string, max int) []string {
	if len(items) <= max {
		return items
	}
	out := append([]string{}, items[:max]...)
	return append(out, "…(+lainnya)")
}
