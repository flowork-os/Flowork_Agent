// chat_prune_ext.go — SEAM (NON-frozen, sibling, DELETABLE). Bersihin chat KOSONG yang
// numpuk ("New chat" tanpa pesan). Tombol "+ New chat" bikin sesi langsung walau belum
// dikirim apa-apa → kalau diklik berkali-kali, sidebar penuh "New chat" yang ga kehapus
// massal. Endpoint ini hapus SEMUA sesi 0-pesan sekaligus (yang ADA isinya AMAN, ga kehapus).
//
// Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com (white-label)
// Daftar route lewat RegisterFeature (registry frozen, registrasi = seam). Hapus file ini →
// fitur prune mati mulus, chat utuh.
//
//	POST /api/chat/sessions/prune  → {ok, deleted, kept}
package main

import (
	"net/http"

	"flowork-gui/internal/floworkdb"
)

// chatSessionsPruneHandler — hapus tiap sesi yang NOL pesan (kosong). Sesi yang udah ada
// percakapannya (>=1 pesan) dipertahankan. Best-effort: gagal hapus 1 ga nyetop sisanya.
func chatSessionsPruneHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		sessions, err := store.ListChatSessions()
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		deleted, kept := 0, 0
		for _, s := range sessions {
			msgs, merr := store.ListChatMessages(s.ID, 1) // cukup 1 buat tau kosong/enggak
			if merr != nil {
				kept++
				continue
			}
			if len(msgs) == 0 {
				if store.DeleteChatSession(s.ID) == nil {
					deleted++
				} else {
					kept++
				}
			} else {
				kept++
			}
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "deleted": deleted, "kept": kept})
	}
}

func init() {
	RegisterFeature(Feature{Name: "chat-prune", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/chat/sessions/prune", chatSessionsPruneHandler(d.FDB))
	}})
}
