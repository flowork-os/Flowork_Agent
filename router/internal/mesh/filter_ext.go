// filter_ext.go — EXTENSION SEAM (NON-frozen, Rule 7) buat RunFilterPipeline (FROZEN di
// karma_toolshare_filter.go). Owner: Aola Sahidin · github.com/flowork-os/Flowork-OS · floworkos.com
//
// AKAR (audit 2026-06-26): filter 9-lapis (L1-L9) dipanggil INLINE di RunFilterPipeline (FROZEN).
// Nambah lapis filter BARU (L10+) = kepaksa buka file frozen → langgar prinsip evolusi. Skill
// (RegisterSkillProvider), graph (RegisterGraphProjection), trigger (RegisterDeliverer) udah punya
// registry-seam; mesh-filter BELUM → ditutup di sini (cabut-akar, bukan tambal).
//
// CARA NAMBAH LAPIS FILTER BARU (zero edit frozen) — bikin file sibling BARU mesh/filter_<x>.go:
//
//	func init() {
//	    RegisterMeshFilter(MeshFilter{
//	        Name:   "geo-fence",
//	        Switch: "FLOWORK_MESH_GEOFENCE",  // tambah entri di agent fwswitch/registry.go → muncul GUI
//	        Run: func(db *sql.DB, pkt Packet, content string) FilterDecision {
//	            // pass/reject/flag — reject = paket di-tolak (pipeline stop di lapis ini)
//	            return FilterDecision{Layer: "L10-geofence", Decision: "pass"}
//	        },
//	    })
//	}
//
// RunFilterPipeline (frozen) manggil runExtraMeshFilters SEKALI sebelum L9-promote → SEMUA lapis
// terdaftar otomatis jalan. Sekali hook, selamanya plug-and-play.
package mesh

import (
	"database/sql"
	"os"
	"strings"
)

// MeshFilter — 1 lapis filter tambahan buat pipeline mesh, didaftarin TANPA buka file frozen.
// Run balikin FilterDecision (Decision "pass"/"reject"/"flag"). Switch kosong = selalu jalan;
// isi ENV key biar bisa dimatiin dari GUI (daftarin juga di agent internal/fwswitch/registry.go).
type MeshFilter struct {
	Name   string
	Switch string
	Run    func(db *sql.DB, pkt Packet, drawerContent string) FilterDecision
}

var extraMeshFilters []MeshFilter

// RegisterMeshFilter — titik-extend RESMI (pola RegisterGraphProjection/RegisterDeliverer).
// Panggil dari init() file sibling BARU. Run==nil diabaikan (no-op aman).
func RegisterMeshFilter(f MeshFilter) {
	if f.Run == nil {
		return
	}
	extraMeshFilters = append(extraMeshFilters, f)
}

// meshFilterSwitchOn — default ON; OFF kalau ENV switch = 0/false/off/no. Switch kosong → ON.
func meshFilterSwitchOn(key string) bool {
	if strings.TrimSpace(key) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// runExtraMeshFilters — dipanggil RunFilterPipeline (frozen) SEKALI sebelum L9-promote. Tiap lapis
// terdaftar yg switch-nya ON dijalanin. Kalau ada yg "reject" → reject=true (pipeline stop).
// FAILS-OPEN: panic 1 lapis di-recover jadi "pass" (lapis rusak GAK robohin pipeline inti).
// Balikin (keputusan-lapis-tambahan, adaReject?).
func runExtraMeshFilters(db *sql.DB, pkt Packet, drawerContent string) ([]FilterDecision, bool) {
	var out []FilterDecision
	reject := false
	for _, f := range extraMeshFilters {
		if !meshFilterSwitchOn(f.Switch) {
			continue
		}
		d := safeRunMeshFilter(f, db, pkt, drawerContent)
		out = append(out, d)
		if d.Decision == "reject" {
			reject = true
		}
	}
	return out, reject
}

func safeRunMeshFilter(f MeshFilter, db *sql.DB, pkt Packet, drawerContent string) (d FilterDecision) {
	defer func() {
		if r := recover(); r != nil {
			d = FilterDecision{Layer: "ext-" + f.Name, Decision: "pass", Reason: "filter panic recovered"}
		}
	}()
	return f.Run(db, pkt, drawerContent)
}
