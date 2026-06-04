// plugin_handler.go — Plug-and-Play Task Packs (roadmap Phase 1-2-4).
//
//	POST /api/plugins/install  (multipart "file" = .fwpack zip)
//	  → validate → CAPS CONSENT gate → extract agent(s) → daftarin kategori+crew
//	    → SMOKE-TEST synth (gagal = kategori di-disable, ga di-expose ke mr-flow).
//	    Kategori kebaca mr-flow classifier (Phase 0 dynamic). Loopback-only.
//
// .fwpack layout (zip):
//
//	plugin.json
//	agents/<agent-id>/{agent.wasm, manifest.json}
//
// CATATAN: SENGAJA ga manggil UploadHandler (stabil) — extract self-contained +
// path-safe (anti zip-slip). Phase 5 CLI / Phase 6 uninstall+dogfood = nanti.

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

var pluginIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

// pluginDangerCapPrefixes — caps yang WAJIB consent owner pas install plugin
// (untrusted). Pakai PRIMITIVE ASLI Flowork (lihat manifest.go: fs/net/kv/exec/
// bus/secret/time/rpc/state). Yang bahaya buat plugin pihak-ketiga:
//   exec:*           → jalanin command / kendali PC (exec:power)
//   secret:*         → baca secret owner (TOKEN exfil!)
//   fs:shared        → akses file warga lain
//   rpc:agent-invoke → setir agent lain
// net:fetch / fs (workspace sendiri) / state / time = normal, ga di-flag.
var pluginDangerCapPrefixes = []string{"exec:", "secret:", "fs:shared", "rpc:agent-invoke"}

func pluginCapDangerous(c string) bool {
	for _, p := range pluginDangerCapPrefixes {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

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

// scanPackCaps — baca manifest tiap agent di pack (dari zip) → kumpulin caps
// berbahaya yang diminta. Dipakai consent gate SEBELUM extract.
func scanPackCaps(zr *zip.Reader) (danger []string) {
	seen := map[string]bool{}
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		if !strings.HasPrefix(name, "agents/") || !strings.HasSuffix(name, "/manifest.json") {
			continue
		}
		rc, e := f.Open()
		if e != nil {
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(rc, 1<<20))
		rc.Close()
		var mf struct {
			CapabilitiesRequired []string `json:"capabilities_required"`
		}
		if json.Unmarshal(raw, &mf) != nil {
			continue
		}
		for _, c := range mf.CapabilitiesRequired {
			if pluginCapDangerous(c) && !seen[c] {
				seen[c] = true
				danger = append(danger, c)
			}
		}
	}
	return danger
}

func pluginInstallHandler(host *kernelhost.Host, store *floworkdb.Store) http.HandlerFunc {
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

		// 1) parse + validate plugin.json
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

		// 2) CAPS CONSENT (Phase 4.1): kalau agent pack minta caps bahaya + owner
		// belum approve (?approve_caps=1) → TOLAK, kasih daftar caps-nya. Default-deny.
		danger := scanPackCaps(zr)
		approved := r.URL.Query().Get("approve_caps") == "1"
		if len(danger) > 0 && !approved {
			tfWriteJSON(w, http.StatusForbidden, map[string]any{
				"error":            "pack minta caps BERBAHAYA — butuh persetujuan owner",
				"dangerous_caps":   danger,
				"approve_hint":     "install ulang dengan query ?approve_caps=1 kalau lo percaya pack ini",
				"plugin":           man.ID,
				"consent_required": true,
			})
			return
		}

		// 3) extract agent ke STAGING dulu (path-safe), lalu ATOMIC RENAME ke
		// AgentsDir/<id>.fwagent → fsnotify liat 1 dir LENGKAP sekaligus → hot-load
		// BERSIH (no partial-write race; kernel watcher ChangeAdded → LoadInstance).
		// Staging di dalam AgentsDir (same FS biar rename atomic), prefix "." + BUKAN
		// ".fwagent" → watcher ignore staging-nya.
		agentsRoot := loader.AgentsDir()
		staging := filepath.Join(agentsRoot, ".plugin-staging-"+man.ID)
		_ = os.RemoveAll(staging)
		defer os.RemoveAll(staging)
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
			stageDir := filepath.Join(staging, aid)
			dest := filepath.Join(stageDir, filepath.FromSlash(rel))
			if c, e := filepath.Rel(stageDir, dest); e != nil || strings.HasPrefix(c, "..") {
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
		// ATOMIC MOVE tiap agent staging → AgentsDir/<id>.fwagent (replace kalau upgrade)
		for aid := range installed {
			finalDir := filepath.Join(agentsRoot, aid+".fwagent")
			_ = os.RemoveAll(finalDir)
			if e := os.Rename(filepath.Join(staging, aid), finalDir); e != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "install agent " + aid + ": " + e.Error()})
				return
			}
		}

		// 4) daftarin kategori + crew (synth = Synthesizer, worker = SetCrew)
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

		// 5) SMOKE-TEST (Phase 4.2): tunggu hot-load synth → ping. Kalau synth
		// GA KE-LOAD (pack broken) → DISABLE kategori biar ga di-expose ke mr-flow.
		// LLM error (loaded tapi hiccup) → tetep enabled (transient, jangan agresif).
		smoke := smokeTestSynth(host, synth)
		if smoke == "not_loaded" {
			cat.Enabled = false // pack broken (synth ga load) → DISABLE, jangan expose ke mr-flow
			_ = store.UpsertCategory(cat)
		}

		tfWriteJSON(w, 0, map[string]any{
			"ok":             smoke != "not_loaded",
			"plugin":         man.ID,
			"category":       cat.ID,
			"enabled":        smoke != "not_loaded",
			"synth":          synth,
			"workers":        len(workers),
			"agents_extract": installed,
			"caps_approved":  len(danger) > 0 && approved,
			"dangerous_caps": danger,
			"smoke":          smoke,
			"next":           "kategori LIVE — mr-flow classifier auto-discover (cache <=60s).",
		})
	}
}

// smokeTestSynth — tunggu synth hot-load (fsnotify) lalu ping. Balik:
//
//	"ok"          → synth load + balas (sehat)
//	"llm_error"   → synth load tapi LLM hiccup (tetep dianggap install OK)
//	"not_loaded"  → synth ga ke-load dalam timeout (pack broken → disable kategori)
func smokeTestSynth(host *kernelhost.Host, synthID string) string {
	if host == nil || synthID == "" {
		return "not_loaded"
	}
	for attempt := 0; attempt < 8; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		reply, err := host.InvokeAgentMessage(ctx, synthID, "PING instalasi — balas singkat 'ok' aja.", "plugin-smoke")
		cancel()
		if err == nil {
			if strings.TrimSpace(reply) != "" {
				return "ok"
			}
			return "llm_error"
		}
		if strings.Contains(err.Error(), "not loaded") {
			time.Sleep(1500 * time.Millisecond) // kasih waktu fsnotify hot-load
			continue
		}
		return "llm_error" // error lain = agent ada tapi gagal jalan (transient)
	}
	return "not_loaded"
}
