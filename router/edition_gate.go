// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah fitur TANPA buka frozen: file sibling baru + registry (RegisterMeshFilter/
// RegisterExtraRoute/RegisterGraphProjection) + SWITCH fwswitch. Pola: lock/frozen-core.md

package main

import (
	"net/http"
	"os"
	"strings"
)

func freeEdition() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("FLOWORK_EDITION")), "corporate")
}

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
