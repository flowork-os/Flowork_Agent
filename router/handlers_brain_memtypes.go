// handlers_brain_memtypes.go — GET /api/brain/mem-types → daftar mem_type buat GUI dropdown.
// AKAR (Bagian 3): dua dropdown (Add Knowledge + Typed Memory) di-hardcode terpisah & beda isi →
// drawer ber-tipe non-kanonik (mis. "knowledge") gak muncul di tab Typed. FIX (GUI=truth):
// satu sumber = AllMemTypes kanonik (mem_type_registry.go) ∪ mem_type yang BENERAN ADA di drawers
// (biar legacy gak ilang). Kedua dropdown baca dari sini → sinkron otomatis. File baru, non-frozen.
package main

import (
	"net/http"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func brainMemTypesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	s, _ := store.LoadSettings(d)
	applyBrainPath(s)

	seen := map[string]bool{}
	out := []string{}
	// 1. kanonik (urutan registry = urutan fungsional)
	for _, mt := range brain.AllMemTypes {
		v := string(mt)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	// 2. yang BENERAN ada di drawers (dinamis, anti-ilang) — best-effort
	if brain.Available() {
		if db, err := brain.OpenRW(); err == nil {
			if rows, qerr := db.QueryContext(r.Context(),
				"SELECT DISTINCT mem_type FROM drawers WHERE mem_type IS NOT NULL AND mem_type <> ''"); qerr == nil {
				for rows.Next() {
					var v string
					if rows.Scan(&v) == nil && v != "" && !seen[v] {
						seen[v] = true
						out = append(out, v)
					}
				}
				rows.Close()
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": out})
}
