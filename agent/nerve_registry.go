// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-27 (F2 roadmap-evolusi).
// LOCKED ≠ FREEZE: boleh diedit DENGAN izin owner. AI lain: JANGAN otak-atik. Mekanisme inti papan
// saraf (dipanggil guard F3 selfevolve_coreapply.go) — BUKAN sibling deletable. Seed/data = sibling.
// 📄 Dok: FLowork_os/lock/peta-saraf.md
//
// nerve_registry.go — PAPAN-COLOKAN SARAF (POLA A). Inti Flowork = TUBUH beku; AI tumbuh CUMA
// lewat 3 saluran: SWITCH (FLOWORK_*) · DATA (brain/mesh) · MODUL (.fwpack). Papan ini = daftar
// resmi "saklar yang AI boleh pakai" — AI tanya `Nerves()` buat tau "tombol apa yang gue punya".
//
// POLA A: papan DIBEKUIN, default KOSONG = aman (kalau ga ada sibling yg nyolok → list kosong,
// inti tetep jalan apa adanya). Saraf dicolok lewat file SIBLING (nerve_seed_ext.go, BISA DIHAPUS).
// F3 pakai NerveChannels() buat nolak usulan di luar 3 saluran (mis. "edit main.go") otomatis.
package main

import (
	"strings"
	"sync"
)

// Nerve — 1 titik-tumbuh yg AI boleh "pencet" tanpa nyentuh kode beku.
type Nerve struct {
	Name    string `json:"name"`    // FLOWORK_X (switch) · RegisterX (registry) · nama data/modul
	Kind    string `json:"kind"`    // switch | registry | data | modul
	Default string `json:"default"` // posisi default aman (kosong/on/off/angka)
	Desc    string `json:"desc"`    // 1 kalimat: dia ngapain
}

// nerveChannels — 3 SALURAN sah perubahan. Di luar ini = HARAM (F3 tolak otomatis).
// registry = sub-bentuk POLA-A dari saluran "switch" (nambah item via sibling _ext).
var nerveChannels = []string{"switch", "data", "modul"}

var (
	nerveMu  sync.RWMutex
	nerveReg []Nerve
	nerveIdx = map[string]int{} // name → index (dedupe + lookup O(1))
)

// nerveKindValid — kind yg diterima papan. registry dipetakan ke saluran "switch".
func nerveKindValid(k string) bool {
	switch k {
	case "switch", "registry", "data", "modul":
		return true
	}
	return false
}

// nerveKindChannel — petakan kind → 1 dari 3 saluran sah (registry→switch).
func nerveKindChannel(kind string) string {
	if kind == "registry" {
		return "switch"
	}
	return kind
}

// RegisterNerve — colok 1 saraf ke papan (dipanggil dari file SIBLING _ext). Fail-safe:
// nama kosong / kind ga sah → diabaikan diam-diam (papan ga pernah korup). Idempotent:
// nama sama → overwrite (boot ulang / re-seed aman).
func RegisterNerve(n Nerve) {
	n.Name = strings.TrimSpace(n.Name)
	n.Kind = strings.ToLower(strings.TrimSpace(n.Kind))
	if n.Name == "" || !nerveKindValid(n.Kind) {
		return
	}
	nerveMu.Lock()
	defer nerveMu.Unlock()
	if i, ok := nerveIdx[n.Name]; ok {
		nerveReg[i] = n
		return
	}
	nerveIdx[n.Name] = len(nerveReg)
	nerveReg = append(nerveReg, n)
}

// Nerves — snapshot daftar saraf (buat query AI / GUI). Salinan → caller ga bisa korup papan.
func Nerves() []Nerve {
	nerveMu.RLock()
	defer nerveMu.RUnlock()
	out := make([]Nerve, len(nerveReg))
	copy(out, nerveReg)
	return out
}

// NerveByName — cari 1 saraf. ok=false kalau ga terdaftar (= AI ga punya tombol itu).
func NerveByName(name string) (Nerve, bool) {
	nerveMu.RLock()
	defer nerveMu.RUnlock()
	if i, ok := nerveIdx[strings.TrimSpace(name)]; ok {
		return nerveReg[i], true
	}
	return Nerve{}, false
}

// NerveCount — jumlah saraf terdaftar.
func NerveCount() int {
	nerveMu.RLock()
	defer nerveMu.RUnlock()
	return len(nerveReg)
}

// NerveChannels — 3 saluran sah (salinan). F3 acuan: kind usulan harus ke salah satu ini.
func NerveChannels() []string { return append([]string(nil), nerveChannels...) }

// NerveChannelValid — true kalau `channel` salah satu dari 3 saluran sah. Dipakai F3 buat
// nolak usulan "edit kode inti" (channel di luar switch/data/modul = HARAM).
func NerveChannelValid(channel string) bool {
	channel = strings.ToLower(strings.TrimSpace(channel))
	for _, c := range nerveChannels {
		if c == channel {
			return true
		}
	}
	return false
}
