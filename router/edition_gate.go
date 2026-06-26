// edition_gate.go — GROWTH-POINT (NON-frozen). #2 BISNIS FREE vs CORPORATE.
//
// AKAR: build FREE (Mr.Flow AS-IS) JANGAN bisa di-rebrand — identitas (persona Aola + konstitusi
// AOLA) WAJIB read-only biar ga ada yg ganti jadi merk lain terus jual ulang. CORPORATE (berbayar)
// = boleh white-label (unlock edit identitas).
//
// MEKANISME: gate WRITE (POST/PUT/DELETE/PATCH) ke endpoint identitas. READ (GET) tetep jalan
// (user FREE tetep bisa LIHAT konstitusi/persona, cuma ga bisa UBAH). Switch FLOWORK_EDITION
// (default FREE = terkunci; "corporate" = unlock). Di-manage GUI Switch Fitur (prefix FLOWORK_).
package main

import (
	"net/http"
	"os"
	"strings"
)

// freeEdition — true kalau build FREE (default). FLOWORK_EDITION=corporate → false (unlock rebrand).
func freeEdition() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("FLOWORK_EDITION")), "corporate")
}

// editionGate — bungkus handler identitas: di FREE, WRITE ditolak 403 (anti-rebrand); READ lolos.
// CORPORATE = full akses. No-op buat GET → aman dipasang ke handler GET+write campur.
func editionGate(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			if freeEdition() {
				writeJSON(w, http.StatusForbidden, map[string]any{
					"error":   "Identitas (persona/konstitusi) DI-LOCK di edisi FREE — Flowork tetep Mr.Flow (Aola Sahidin). Upgrade ke CORPORATE buat white-label / rebrand.",
					"edition": "free",
					"locked":  true,
				})
				return
			}
		}
		h(w, r)
	}
}
