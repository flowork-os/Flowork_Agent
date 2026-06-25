package main

// === FROZEN === (chattr +i + hash KERNEL_FREEZE.md, 2026-06-25). Logika inti
// anti-flailing — STABIL (proven 4/4 unit test). Tuning threshold (flailWindow/
// flailDupMax/maxFlailNudges) = unfreeze SADAR + izin owner. Lihat lock/ERROR_EDUKASI.md.
//
// flail_guard.go — ANTI-FLAILING (mantok) di tool-loop. Pasangan ghost-guard, beda kasus:
//
//   ghost-guard  = model NARASI niat tanpa manggil tool (janji kosong).
//   flail-guard  = model MANGGIL tool SAMA berulang TANPA progress (mantok/muter).
//
// AKAR (owner 2026-06-25): mr-flow ditanya "cek perubahan repo" → manggil `file_list`
// ~20× (kebanyakan args SAMA) → lolos SEMUA guard: bukan narasi (ghost-guard lewat),
// bukan ERROR (captureRecovery lewat), masih < budget-waktu & maxToolIters. Error-edukasi
// cuma nangkep ERROR, GAK nangkep FLAILING (sukses-tapi-sia-sia berulang). Owner:
// "looping 20× = error-edukasi ga work; tiap kesalahan ada pembelajaran."
//
// DETEKSI = window-duplikat (BUKAN sekadar "tool sama beruntun" — itu false-positive
// di kerja sah kayak baca 10 file beda). Sinyal flail yang BERSIH: signature `tool|args`
// yang SAMA PERSIS muncul ≥flailDupMax kali dalam flailWindow call terakhir. Nangkep
// repeat beruntun (job×15) DAN cycling (job/tools/log/job...), tapi NOL false-positive
// di batch args-beda.
//
// AKSI (hormati owner "loop jangan dibatasi"): BUKAN hard-stop — inject koreksi KERAS
// (redirect ke tool lain / tool_search / ScheduleWakeup / kasih hasil), bounded
// maxFlailNudges. Kalau MASIH mantok lewat batas → eskalasi JUJUR ke owner (stuck
// beneran → tanya, bukan ngarang/muter).

const (
	flailWindow    = 8 // lihat N tool-call terakhir
	flailDupMax    = 3 // signature SAMA muncul ≥N kali dalam window → flailing
	maxFlailNudges = 4 // bounded koreksi (mirror maxGhostNudges)
)

// flailState — di-track per turn (scope tool-loop). main.go yang nyimpen + manggil.
type flailState struct {
	window []string // ring signature `tool|args` dari call terakhir
	nudges int      // berapa kali udah dikoreksi (bounded)
}

// check — update state dengan call sekarang, balik keputusan:
//   - nudge=true    → main.go inject `corrective` sbg pesan user, lanjut loop (bounded).
//   - escalate=true → udah dikoreksi maxFlailNudges× tapi MASIH mantok → main.go
//     eskalasi jujur ke owner (return flailEscalation), STOP turn dgn jujur.
//   - dua-duanya false → normal, lanjut.
func (f *flailState) check(toolName, argsJSON string) (corrective string, nudge bool, escalate bool) {
	sig := toolName + "|" + argsJSON
	f.window = append(f.window, sig)
	if len(f.window) > flailWindow {
		f.window = f.window[len(f.window)-flailWindow:]
	}
	dup := 0
	for _, s := range f.window {
		if s == sig {
			dup++
		}
	}
	if dup < flailDupMax {
		return "", false, false
	}
	if f.nudges >= maxFlailNudges {
		return "", false, true // udah dikoreksi cukup, tetep mantok → eskalasi
	}
	f.nudges++
	f.window = f.window[:0] // reset window: kasih kesempatan fresh abis dikoreksi
	return flailCorrective(toolName), true, false
}

// flailCorrective — koreksi deterministik pas mantok ke-deteksi. Konkret + kasih
// PILIHAN aksi (gaya cheat-sheet: redirect, bukan cuma "stop").
func flailCorrective(toolName string) string {
	return "⚠️ STOP — lo udah manggil tool `" + toolName + "` berkali-kali dengan hasil yang SAMA / ga maju (mantok). Ngulang cara yang sama = buang waktu + owner nunggu sia-sia. PILIH SEKARANG salah satu: (a) pakai tool LAIN yang lebih tepat buat tujuanmu; (b) kalau ga yakin tool mana yang ada, panggil `tool_search` buat nyari; (c) kalau emang lagi NUNGGU sesuatu yang belum siap, panggil `ScheduleWakeup`; (d) kalau infonya udah cukup, kasih hasil-sejauh-ini ke owner. JANGAN ulang `" + toolName + "` dengan args yang sama lagi."
}

// flailEscalation — pesan JUJUR pas model tetep mantok lewat batas koreksi. Eskalasi
// ke owner (minta arahan), BUKAN ngarang udah-kelar / muter selamanya.
func flailEscalation(toolName string) string {
	return "🛑 Gw jujur: mantok di `" + toolName + "` — udah dicoba berkali-kali + udah gw koreksi sendiri tapi belum nemu jalan. Gw stop di sini biar ga muter terus + buang resource. Butuh arahan lo: tujuannya apa persisnya, atau ada tool/cara lain yang lo tau buat ini?"
}
