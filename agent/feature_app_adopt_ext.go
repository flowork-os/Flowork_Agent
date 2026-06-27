// feature_app_adopt_ext.go — SEAM (NON-frozen, DELETABLE). Endpoint "repo → app"
// (ROADMAP_REPO_TO_APP F3): adopt repo (clone/folder → app LIVE) + detect (preview).
// Daftar sendiri lewat init()→RegisterFeature — main.go frozen GA disentuh. Hapus file ini → fitur
// adopt mati mulus, core utuh (self-sufficient).
//
// Consent: adopt jalanin perintah OS (clone+install) → wajib ?approve_exec=1 (owner buka gerbang).
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"flowork-gui/internal/apps"
)

func init() {
	RegisterFeature(Feature{Name: "apps-adopt", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/apps/detect", appsDetectHandler())
		d.Mux.HandleFunc("/api/apps/adopt", appsAdoptHandler(d.AppsMgr))
	}})
}

// POST /api/apps/detect {source} — PREVIEW runtime (dry-run, no install, no go-live).
func appsDetectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var b struct {
			Source string `json:"source"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&b); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
		defer cancel()
		det, err := apps.DetectSource(ctx, b.Source)
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{
			"ok": true, "suggested_id": apps.SlugID(b.Source), "detection": det,
		})
	}
}

// POST /api/apps/adopt {source, id?, force?} ?approve_exec=1 — clone/copy → install → app LIVE.
func appsAdoptHandler(mgr *apps.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var b struct {
			Source      string `json:"source"`
			ID          string            `json:"id"`
			Force       bool              `json:"force"`
			SkipInstall bool              `json:"skip_install"`
			Contract    string            `json:"contract"` // ""/"cli" = CLI · "http" = server (web app/API)
			HTTP        apps.HTTPContract `json:"http"`     // dipakai kalau contract=http
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&b); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		approveExec := r.URL.Query().Get("approve_exec") == "1"
		if !approveExec {
			tfWriteJSON(w, http.StatusForbidden, map[string]any{
				"error":            "adopt jalanin perintah OS (clone+install) — butuh persetujuan owner",
				"consent_required": true,
				"approve_hint":     "ulang dengan ?approve_exec=1 kalau lo percaya repo ini",
			})
			return
		}
		// adopt bisa lama (clone+install) — kasih konteks timeout panjang, lepas dari client.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		var res apps.AdoptResult
		var err error
		if b.Contract == "http" {
			res, err = mgr.AdoptHTTPRepo(ctx, b.Source, strings.TrimSpace(b.ID), b.HTTP, true, b.SkipInstall, b.Force)
		} else {
			res, err = mgr.AdoptRepo(ctx, b.Source, strings.TrimSpace(b.ID), true, b.SkipInstall, b.Force)
		}
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "partial": res})
			return
		}
		next := "app LIVE — buka di tab App / panggil tool app_" + res.ID + "_run"
		if b.Contract == "http" {
			next = "app SERVER LIVE — buka UI via op _url (tab App), atau panggil op HTTP-nya"
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "result": res, "next": next})
	}
}
