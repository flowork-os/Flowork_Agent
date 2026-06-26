// graph_extras_ext.go — EXTENSION SEAM (NON-frozen, Rule 7) buat graph_extras.go (FROZEN).
// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
//
// AKAR (audit 2026-06-26): proyeksi DreamGraph (instincts/knowledge-hubs) dipanggil INLINE di
// SyncGraphExtended (FROZEN). Mau nambah proyeksi BARU = kepaksa buka file frozen. Skill udah punya
// RegisterSkillProvider; graph-projection BELUM. File ini nutup gap (cabut-akar, bukan tambal).
//
// CARA NAMBAH PROYEKSI BARU (zero edit file frozen) — bikin file sibling BARU:
//
//	func init() {
//	    RegisterGraphProjection(GraphProjection{
//	        Name:   "xxx",
//	        Switch: "FLOWORK_DREAMGRAPH_XXX",     // tambah entri di fwswitch/registry.go → muncul GUI
//	        Run:    syncXxxToGraph,               // func(ctx, tx) (int, error), idempotent
//	    })
//	}
//
// SyncGraphExtended (frozen) cukup manggil runExtraGraphProjectionsTx SEKALI sebelum RAG-mirror →
// SEMUA proyeksi terdaftar otomatis ke-run dalam tx yg sama. Sekali hook, selamanya plug-and-play.
package brain

import (
	"context"
	"database/sql"
)

// GraphProjection — 1 proyeksi tambahan DreamGraph (router), jalan DALAM tx SyncGraphExtended.
// Run WAJIB idempotent (cleanup-source dulu lalu insert, pola syncInstinctsToGraph) + mirror-only
// (JANGAN hapus sumber). Switch kosong = selalu jalan; isi ENV key biar bisa dimatiin dari GUI.
type GraphProjection struct {
	Name   string
	Switch string
	Run    func(ctx context.Context, tx *sql.Tx) (int, error)
}

var extraGraphProjections []GraphProjection

// RegisterGraphProjection — titik-extend RESMI (pola RegisterSkillProvider). Panggil dari init()
// file sibling BARU. Run==nil diabaikan (no-op aman).
func RegisterGraphProjection(p GraphProjection) {
	if p.Run == nil {
		return
	}
	extraGraphProjections = append(extraGraphProjections, p)
}

// runExtraGraphProjectionsTx — dipanggil SyncGraphExtended (frozen) sebelum RAG-mirror. Tiap proyeksi
// yg switch-nya ON (extraSwitchOn, default ON; Switch kosong → selalu) jalan dalam tx yg sama.
// FAILS-OPEN: error 1 proyeksi di-skip, gak nge-abort core DreamGraph sync (yg lama TIDAK rusak).
// Balikin total node baru/berubah.
func runExtraGraphProjectionsTx(ctx context.Context, tx *sql.Tx) int {
	total := 0
	for _, p := range extraGraphProjections {
		if p.Switch != "" && !extraSwitchOn(p.Switch) {
			continue
		}
		n, err := p.Run(ctx, tx)
		if err == nil {
			total += n
		}
	}
	return total
}
