// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-27 (F4 roadmap-evolusi).
// LOCKED ≠ FREEZE: boleh diedit DENGAN izin owner. CATATAN: recordButuhTombol dipanggil guard frozen
// selfevolve_coreapply.go → JANGAN hapus. (Endpoint + store antrian owner.)
// 📄 Dok: FLowork_os/lock/peta-saraf.md
//
// nerve_butuh_tombol_ext.go — F4 "lapor butuh_tombol".
// Pas AI mentok (usulan di luar ruang saraf, ditolak guard F3) → BUKAN bongkar inti, tapi
// LAPOR ke antrian owner: butuh_tombol{lokasi, alasan}. Owner yang nambahin saklar (jarang,
// sadar). Disimpen di KV (floworkdb) — additive, no schema-edit. Endpoint GET/DELETE buat GUI.
package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
)

const (
	butuhTombolKVKey = "evolve_butuh_tombol"
	butuhTombolMax   = 200 // cap antrian (anti unbounded)
)

// ButuhTombol — 1 laporan "AI mentok, butuh saklar baru" buat owner.
type ButuhTombol struct {
	Lokasi    string `json:"lokasi"`     // file/area yg AI pengen sentuh tapi ga ada saklarnya
	Alasan    string `json:"alasan"`     // kenapa (rationale usulan)
	Kind      string `json:"kind"`       // kind usulan asli (fix/refactor/...)
	Channel   string `json:"channel"`    // hasil klasifikasi (core-edit/unknown)
	CreatedAt string `json:"created_at"` // RFC3339 UTC
}

// butuhTombolMerge — PURE: dedupe (lokasi+alasan) + append + cap. Tanpa I/O → gampang dites.
func butuhTombolMerge(list []ButuhTombol, n ButuhTombol) []ButuhTombol {
	n.Lokasi = strings.TrimSpace(n.Lokasi)
	n.Alasan = strings.TrimSpace(n.Alasan)
	if n.Lokasi == "" && n.Alasan == "" {
		return list // kosong → ga dicatat
	}
	for _, b := range list {
		if b.Lokasi == n.Lokasi && b.Alasan == n.Alasan {
			return list // udah ada → ga dobel
		}
	}
	list = append(list, n)
	if len(list) > butuhTombolMax {
		list = list[len(list)-butuhTombolMax:]
	}
	return list
}

func butuhTombolLoad() []ButuhTombol {
	db, err := floworkdb.Shared()
	if err != nil {
		return nil
	}
	v, _ := db.GetKV(butuhTombolKVKey)
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var list []ButuhTombol
	_ = json.Unmarshal([]byte(v), &list)
	return list
}

// recordButuhTombol — catat 1 laporan ke antrian owner. Best-effort: DB error → diam (ga
// ganggu alur evolusi). Dipanggil dari guard F3 (selfevolve_coreapply.go) pas usulan ditolak.
func recordButuhTombol(lokasi, alasan, kind, channel string) {
	db, err := floworkdb.Shared()
	if err != nil {
		return
	}
	merged := butuhTombolMerge(butuhTombolLoad(), ButuhTombol{
		Lokasi: lokasi, Alasan: alasan, Kind: kind, Channel: channel,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	b, _ := json.Marshal(merged)
	_ = db.SetKV(butuhTombolKVKey, string(b))
}

func init() {
	RegisterFeature(Feature{Name: "butuh-tombol", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/evolve/butuh-tombol", butuhTombolHandler())
	}})
}

// butuhTombolHandler — GET: antrian laporan buat panel owner. DELETE: owner kosongin
// (udah dipasangin tombol). Loopback/owner-gated kayak handler evolve lain.
func butuhTombolHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			if db, err := floworkdb.Shared(); err == nil {
				_ = db.SetKV(butuhTombolKVKey, "[]")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "cleared": true})
			return
		}
		list := butuhTombolLoad()
		_ = json.NewEncoder(w).Encode(map[string]any{"count": len(list), "items": list})
	}
}
