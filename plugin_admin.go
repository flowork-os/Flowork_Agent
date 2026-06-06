// plugin_admin.go — Plug-and-Play Phase 6: uninstall + export (dogfood/share).
//
//	POST /api/plugins/uninstall?category=<id>  → cabut kategori+crew, hapus agent
//	     dir yang GA dipake kategori lain (agent shared = di-keep). Loopback-only.
//	GET  /api/plugins/export?category=<id>      → bungkus kategori+crew jadi .fwpack
//	     (download). HANYA manifest.json + agent.wasm + go.mod — NO workspace/state.db
//	     (token aman). Bikin built-in bisa di-share / dogfood. Loopback-only.

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
)

// exportPersona — baca persona (kv.prompt) agent buat dibawa ke .fwpack.
// CUMA prompt — read-only, NO secrets/token (lihat GetPrompt). "" kalau gagal/
// kosong (export tetep jalan, persona opsional).
func exportPersona(agentsRoot, agentID string) string {
	// Resolve = path state.db yang SAMA dipakai agent runtime (prefer source
	// tree, fallback staged) → persona yang ke-export = yang beneran aktif.
	st, err := agentdb.Open(agentdb.Resolve(agentID, filepath.Join(agentsRoot, agentID+".fwagent")))
	if err != nil {
		return ""
	}
	defer st.Close()
	p, _ := st.GetPrompt()
	return p
}

// agentsUsedByOthers — set agent_id yang dipake kategori SELAIN exceptID (synth +
// worker). Dipakai uninstall biar ga hapus agent yang masih dipake kategori lain.
func agentsUsedByOthers(store *floworkdb.Store, exceptID string) map[string]bool {
	used := map[string]bool{}
	cats, err := store.ListCategories()
	if err != nil {
		return used
	}
	for _, o := range cats {
		if o.ID == exceptID {
			continue
		}
		oc, err := store.GetCategory(o.ID)
		if err != nil || oc == nil {
			continue
		}
		if oc.Synthesizer != "" {
			used[oc.Synthesizer] = true
		}
		for _, c := range oc.Crew {
			used[c.AgentID] = true
		}
	}
	return used
}

// uninstallCategoryCore — CABUT 1 kategori + crew + hapus agent dir yang GA
// dipake kategori lain (shared-aware). Dipakai HTTP uninstall handler DAN REAPER
// (apoptosis) — satu jalur, no duplikasi (catatan keras: reuse pipeline).
// Balik (body, httpStatus). status 0 = sukses.
func uninstallCategoryCore(store *floworkdb.Store, catID string) (map[string]any, int) {
	if !pluginIDRe.MatchString(catID) {
		return map[string]any{"error": "category invalid"}, http.StatusBadRequest
	}
	cat, err := store.GetCategory(catID)
	if err != nil {
		return map[string]any{"error": err.Error()}, http.StatusInternalServerError
	}
	if cat == nil {
		return map[string]any{"error": "kategori ga ada"}, http.StatusNotFound
	}
	mine := map[string]bool{}
	if cat.Synthesizer != "" {
		mine[cat.Synthesizer] = true
	}
	for _, c := range cat.Crew {
		mine[c.AgentID] = true
	}
	used := agentsUsedByOthers(store, catID)
	if err := store.DeleteCategory(catID); err != nil {
		return map[string]any{"error": "delete category: " + err.Error()}, http.StatusInternalServerError
	}
	removed, keptShared := []string{}, []string{}
	for aid := range mine {
		if used[aid] {
			keptShared = append(keptShared, aid)
			continue
		}
		_ = os.RemoveAll(filepath.Join(loader.AgentsDir(), aid+".fwagent"))
		removed = append(removed, aid)
	}
	return map[string]any{
		"ok":                 true,
		"category":           catID,
		"agents_removed":     removed,
		"agents_kept_shared": keptShared,
	}, 0
}

func pluginUninstallHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		body, status := uninstallCategoryCore(store, r.URL.Query().Get("category"))
		tfWriteJSON(w, status, body)
	}
}

func pluginExportHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		catID := r.URL.Query().Get("category")
		if !pluginIDRe.MatchString(catID) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "category invalid"})
			return
		}
		cat, err := store.GetCategory(catID)
		if err != nil || cat == nil {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": "kategori ga ada"})
			return
		}
		// bangun plugin.json dari kategori+crew
		var man pluginManifest
		man.ID = catID + "-pack"
		man.Name = cat.Name
		man.Version = "1.0.0"
		man.Author = "flowork-export"
		man.Category.ID = cat.ID
		man.Category.Name = cat.Name
		man.Category.Icon = cat.Icon
		man.Category.TriggerHint = cat.TriggerHint
		man.Category.SynthDirective = cat.SynthDirective
		man.Category.WorkerDirective = cat.WorkerDirective
		agentsRoot := loader.AgentsDir()
		agentIDs := []string{}
		if cat.Synthesizer != "" {
			man.Crew = append(man.Crew, pluginCrewMember{AgentID: cat.Synthesizer, RoleLabel: "synthesizer", Kind: "synth", Persona: exportPersona(agentsRoot, cat.Synthesizer)})
			agentIDs = append(agentIDs, cat.Synthesizer)
		}
		for _, c := range cat.Crew {
			man.Crew = append(man.Crew, pluginCrewMember{AgentID: c.AgentID, RoleLabel: c.RoleLabel, Kind: "worker", Persona: exportPersona(agentsRoot, c.AgentID)})
			agentIDs = append(agentIDs, c.AgentID)
		}

		// zip: plugin.json + agents/<id>/{manifest.json, agent.wasm, go.mod}
		// SENGAJA cuma 3 file source — NO workspace/state.db (token owner aman).
		// Persona ("jiwa" app) ikut lewat plugin.json (bukan state.db) → token tetap aman.
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		pj, _ := json.MarshalIndent(man, "", "  ")
		if f, e := zw.Create("plugin.json"); e == nil {
			_, _ = f.Write(pj)
		}
		for _, aid := range agentIDs {
			dir := filepath.Join(agentsRoot, aid+".fwagent")
			for _, fn := range []string{"manifest.json", "agent.wasm", "go.mod"} {
				data, e := os.ReadFile(filepath.Join(dir, fn))
				if e != nil {
					continue
				}
				if zf, e := zw.Create("agents/" + aid + "/" + fn); e == nil {
					_, _ = zf.Write(data)
				}
			}
		}
		if err := zw.Close(); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "zip: " + err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+catID+`.fwpack"`)
		_, _ = w.Write(buf.Bytes())
	}
}
