// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-27 (F3 roadmap-evolusi).
// LOCKED ≠ FREEZE: boleh diedit DENGAN izin owner. CATATAN: walau nama _ext, ini logika GATE inti
// yg dipanggil guard frozen selfevolve_coreapply.go → JANGAN hapus (bukan sibling buang-an).
// 📄 Dok: FLowork_os/lock/peta-saraf.md
//
// nerve_proposal_ext.go — KLASIFIKASI usulan evolusi ke 3
// SALURAN SARAF (switch/data/modul) buat F3 "kunci pengusul ke ruang saraf". Logika MURNI
// (no I/O, no state) → gampang dites + aman. Penjaga di selfevolve_coreapply.go yang MANGGIL
// helper ini (wire-in = keputusan owner, file itu soft-locked).
//
// Pemetaan (dari definisi roadmap EVOLUSI):
//   add-skill              → DATA  (pengetahuan/skill ditambah ke brain)
//   add-agent | add-app    → MODUL (.fwpack behavior artifact lewat AI Studio)
//   set-switch             → SWITCH (pencet saklar GUI; target wajib saraf terdaftar)
//   fix|refactor|doc|test  → CORE-EDIT = BUKAN saluran saraf → TOLAK (lapor butuh_tombol/upstream)
package main

import "strings"

// NerveProposalVerdict — hasil klasifikasi 1 usulan evolusi.
type NerveProposalVerdict struct {
	Channel string `json:"channel"` // switch | data | modul | core-edit | unknown
	Allowed bool   `json:"allowed"` // true = di dalam ruang saraf (boleh lanjut)
	Reason  string `json:"reason"`  // penjelasan edukatif (buat AI / log)
}

// nerveClassifyKind — petakan kind usulan → saluran saraf. isNerve=false → di luar 3 saluran.
func nerveClassifyKind(kind string) (channel string, isNerve bool, reason string) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "add-skill":
		return "data", true, "skill = pengetahuan ditambah ke brain (saluran DATA)"
	case "add-agent", "add-app":
		return "modul", true, "agent/app = modul behavior (.fwpack lewat AI Studio)"
	case "set-switch", "switch", "pencet-saklar":
		return "switch", true, "pencet saklar GUI (saluran SWITCH)"
	case "fix", "refactor", "doc", "test":
		return "core-edit", false, "edit kode repo/inti BUKAN saluran saraf — pakai switch (saklar GUI), " +
			"data (add-skill), atau modul (add-agent/add-app); kalau butuh seam baru → lapor butuh_tombol ke owner"
	}
	return "unknown", false, "kind '" + strings.TrimSpace(kind) + "' di luar 3 saluran saraf (switch/data/modul)"
}

// NerveProposalVet — verdict 1 usulan (kind + target). Buat saluran SWITCH, target WAJIB saraf
// yang terdaftar di papan (NerveByName) — usul pencet saklar yg ga ada = ditolak (ngarang).
func NerveProposalVet(kind, targetFile string) NerveProposalVerdict {
	channel, isNerve, reason := nerveClassifyKind(kind)
	if !isNerve {
		return NerveProposalVerdict{Channel: channel, Allowed: false, Reason: reason}
	}
	if channel == "switch" {
		name := strings.TrimSpace(strings.TrimPrefix(targetFile, "switch:"))
		if _, ok := NerveByName(name); !ok {
			return NerveProposalVerdict{Channel: channel, Allowed: false,
				Reason: "saklar '" + name + "' TIDAK terdaftar di papan saraf — lapor butuh_tombol, jangan ngarang"}
		}
	}
	return NerveProposalVerdict{Channel: channel, Allowed: true, Reason: reason}
}
