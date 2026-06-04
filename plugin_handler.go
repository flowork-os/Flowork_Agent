// plugin_handler.go — Plug-and-Play Task Packs (roadmap Phase 1-2).
//
//	POST /api/plugins/install  (multipart "file" = .fwpack zip)
//	  → extract agent(s) ke AgentsDir + daftarin kategori+crew dari plugin.json.
//	    Kategori LANGSUNG kebaca mr-flow classifier (Phase 0 dynamic, cache <=60s).
//	    Loopback-only (lihat whitelist auth middleware).
//
// .fwpack layout (zip):
//
//	plugin.json
//	agents/<agent-id>/{agent.wasm, manifest.json}
//
// CATATAN: SENGAJA ga manggil UploadHandler (stabil) — extract di sini
// self-contained + path-safe (anti zip-slip) biar jalur stabil ga kesentuh.
// Caps consent + smoke-test = roadmap Phase 4 (sekarang auto-approve, single-owner).

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
)

var pluginIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

type pluginCrewMember struct {
	AgentID   string `json:"agent_id"`
	RoleLabel string `json:"role_label"`
	Kind      string `json:"kind"` // "worker" | "synth"
}

type pluginManifest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Author   string `json:"author"`
	Category struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Icon           string `json:"icon"`
		TriggerHint    string `json:"trigger_hint"`
		SynthDirective string `json:"synth_directive"`
	} `json:"category"`
	Crew []pluginCrewMember `json:"crew"`
}

// validate — tolak pack ngaco SEBELUM nyentuh disk/DB. Balik "" kalau valid.
func (m *pluginManifest) validate() string {
	if !pluginIDRe.MatchString(m.ID) {
		return "plugin.id invalid (^[a-z0-9][a-z0-9_-]{1,63}$)"
	}
	if !pluginIDRe.MatchString(m.Category.ID) {
		return "category.id invalid"
	}
	if len(m.Crew) == 0 {
		return "crew kosong"
	}
	synthCount := 0
	for _, c := range m.Crew {
		if !pluginIDRe.MatchString(c.AgentID) {
			return "crew agent_id invalid: " + c.AgentID
		}
		if c.Kind == "synth" {
			synthCount++
		}
	}
	if synthCount != 1 {
		return "crew WAJIB tepat 1 synth, ada " + strconv.Itoa(synthCount)
	}
	return ""
}

func pluginInstallHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "parse form: " + err.Error()})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "missing file field"})
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(io.LimitReader(file, 128<<20))
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "read: " + err.Error()})
			return
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "not a valid zip: " + err.Error()})
			return
		}

		// 1) cari + parse plugin.json (root atau 1 level)
		var manRaw []byte
		for _, f := range zr.File {
			base := strings.TrimPrefix(f.Name, "./")
			if base == "plugin.json" || strings.HasSuffix(base, "/plugin.json") {
				rc, e := f.Open()
				if e == nil {
					manRaw, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
					rc.Close()
				}
				break
			}
		}
		if manRaw == nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "plugin.json ga ketemu di pack"})
			return
		}
		var man pluginManifest
		if err := json.Unmarshal(manRaw, &man); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "plugin.json parse: " + err.Error()})
			return
		}
		if msg := man.validate(); msg != "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "manifest invalid: " + msg})
			return
		}

		// 2) extract agent dirs (agents/<id>/...) → AgentsDir/<id>.fwagent/ (path-safe)
		agentsRoot := loader.AgentsDir()
		installed := map[string]int{}
		for _, f := range zr.File {
			name := strings.TrimPrefix(f.Name, "./")
			if !strings.HasPrefix(name, "agents/") || strings.HasSuffix(name, "/") {
				continue
			}
			rest := strings.TrimPrefix(name, "agents/")
			slash := strings.IndexByte(rest, '/')
			if slash <= 0 {
				continue
			}
			aid, rel := rest[:slash], rest[slash+1:]
			if !pluginIDRe.MatchString(aid) || rel == "" {
				continue
			}
			targetDir := filepath.Join(agentsRoot, aid+".fwagent")
			dest := filepath.Join(targetDir, filepath.FromSlash(rel))
			if c, e := filepath.Rel(targetDir, dest); e != nil || strings.HasPrefix(c, "..") {
				continue // anti zip-slip
			}
			if e := os.MkdirAll(filepath.Dir(dest), 0o755); e != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "mkdir: " + e.Error()})
				return
			}
			rc, e := f.Open()
			if e != nil {
				continue
			}
			out, e := os.Create(dest)
			if e != nil {
				rc.Close()
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "create: " + e.Error()})
				return
			}
			_, _ = io.Copy(out, io.LimitReader(rc, 64<<20))
			out.Close()
			rc.Close()
			installed[aid]++
		}

		// 3) daftarin kategori + crew (synth = field Synthesizer, worker = SetCrew)
		synth := ""
		var workers []floworkdb.TaskAgent
		for i, c := range man.Crew {
			if c.Kind == "synth" {
				synth = c.AgentID
				continue
			}
			workers = append(workers, floworkdb.TaskAgent{
				AgentID: c.AgentID, RoleLabel: c.RoleLabel, OrderIdx: i, Mode: "seq",
			})
		}
		cat := floworkdb.TaskCategory{
			ID: man.Category.ID, Name: man.Category.Name, Icon: man.Category.Icon,
			TriggerHint: man.Category.TriggerHint, Synthesizer: synth,
			SynthDirective: man.Category.SynthDirective, Enabled: true,
		}
		if err := store.UpsertCategory(cat); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "upsert category: " + err.Error()})
			return
		}
		if err := store.SetCrew(cat.ID, workers); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "set crew: " + err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{
			"ok":             true,
			"plugin":         man.ID,
			"category":       cat.ID,
			"synth":          synth,
			"workers":        len(workers),
			"agents_extract": installed,
			"next":           "kategori LIVE — mr-flow classifier auto-discover (cache <=60s); agent hot-reload kernel.",
		})
	}
}
