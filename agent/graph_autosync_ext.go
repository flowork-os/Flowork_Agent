// graph_autosync_ext.go — EXTENSION SEAM (NON-frozen, Rule 7) buat graph_autosync.go (FROZEN).
// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
//
// AKAR (audit 2026-06-26): proyeksi sumber→Cognitive-Graph agent (skills/constitution/edu/drawer/
// codemap/deadletter/orphan) dipanggil INLINE di SyncSourcesToGraph (FROZEN). Mau nambah SUMBER
// proyeksi BARU = kepaksa buka file frozen → langgar prinsip "nambah fitur ga boleh sentuh kramat".
// Skill (RegisterSkillProvider) & Instinct (RegisterInstinctSelector) udah punya registry-seam;
// graph-projection BELUM. File ini nutup gap itu (cabut-akar, bukan tambal).
//
// CARA NAMBAH PROYEKSI BARU (zero edit file frozen) — bikin file sibling BARU, mis. graph_proj_xxx.go:
//
//	func init() {
//	    RegisterGraphProjection(GraphProjection{
//	        Name:   "xxx",
//	        Switch: "FLOWORK_CGM_XXX",            // tambah entri di internal/fwswitch/registry.go → muncul GUI
//	        Run: func(ctx context.Context, store *agentdb.Store, scope string) (int, error) {
//	            return store.SyncXxxToGraph(scope) // idempotent + fails-open
//	        },
//	    })
//	}
//
// Dispatcher frozen (SyncSourcesToGraph) cukup manggil runExtraGraphProjections SEKALI di akhir →
// SEMUA proyeksi terdaftar otomatis ke-run. Sekali hook, selamanya plug-and-play.
package main

import (
	"context"
	"os"
	"strings"

	"flowork-gui/internal/agentdb"
)

// GraphProjection — 1 sumber proyeksi ke Cognitive Graph agent, didaftarin TANPA buka file frozen.
// Run WAJIB idempotent (upsert) + fails-open (error → balik 0,err; JANGAN panik). Switch kosong =
// selalu jalan; isi ENV key biar bisa dimatiin dari GUI (daftarin juga di fwswitch/registry.go).
type GraphProjection struct {
	Name   string
	Switch string
	Run    func(ctx context.Context, store *agentdb.Store, scope string) (int, error)
}

var extraGraphProjections []GraphProjection

// RegisterGraphProjection — titik-extend RESMI (pola RegisterSkillProvider / RegisterInstinctSelector).
// Panggil dari init() file sibling BARU. Run==nil diabaikan (no-op aman).
func RegisterGraphProjection(p GraphProjection) {
	if p.Run == nil {
		return
	}
	extraGraphProjections = append(extraGraphProjections, p)
}

// graphProjectionSwitchOn — default ON; OFF kalau ENV switch = 0/false/off/no. Switch kosong → ON.
func graphProjectionSwitchOn(key string) bool {
	if strings.TrimSpace(key) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// runExtraGraphProjections — dipanggil dispatcher frozen (SyncSourcesToGraph) SEKALI di akhir.
// Tiap proyeksi terdaftar yg switch-nya ON dijalanin. FAILS-OPEN: error 1 proyeksi di-skip, gak
// ganggu core / proyeksi lain (prinsip: nambah fitur APAPUN, yg lama TIDAK rusak). Balikin total
// node baru/berubah.
func runExtraGraphProjections(ctx context.Context, store *agentdb.Store, scope string) int {
	total := 0
	for _, p := range extraGraphProjections {
		if !graphProjectionSwitchOn(p.Switch) {
			continue
		}
		n, err := p.Run(ctx, store, scope)
		if err == nil {
			total += n
		}
	}
	return total
}
