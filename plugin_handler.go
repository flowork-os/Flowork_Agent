// plugin_handler.go — Plug-and-Play Task Packs (roadmap Phase 1-2-4-3).
//
//	POST /api/plugins/install  (multipart "file" = .fwpack zip)
//	  → validate → CAPS CONSENT gate → extract agent(s) (staging+atomic-rename →
//	    hot-load) → daftarin kategori+crew → SMOKE-TEST synth (gagal = disable).
//	    Kategori kebaca mr-flow classifier (Phase 0 dynamic). Loopback-only.
//
// Core = installPluginPack(raw, approveCaps): dipakai handler HTTP DAN watcher
// drop-folder (plugin_watcher.go) — satu jalur, no duplikasi.
//
// .fwpack layout (zip): plugin.json + agents/<id>/{agent.wasm, manifest.json}
//
// CATATAN: SENGAJA ga manggil UploadHandler (stabil) — extract self-contained +
// path-safe (anti zip-slip), staging+rename biar watcher hot-load 1 dir lengkap.

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
// (untrusted). Pakai PRIMITIVE ASLI Flowork (manifest.go: fs/net/kv/exec/bus/
// secret/time/rpc/state). Bahaya buat plugin pihak-ketiga:
//
//	exec:*           → jalanin command / kendali PC (exec:power)
//	secret:*         → baca secret owner (TOKEN exfil!)
//	fs:shared        → akses file warga lain
//	rpc:agent-invoke → setir agent lain
//
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

// scanPackCaps — baca manifest tiap agent di pack → kumpulin caps berbahaya.
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

// pluginInstallResult — hasil core install (status 0 = sukses/200).
type pluginInstallResult struct {
	status int
	body   map[string]any
}

// installPluginPack — CORE install (dipakai HTTP handler + watcher drop-folder).
// approveCaps=true → lewatin consent gate (owner-trusted, mis. drop-folder).
func installPluginPack(host *kernelhost.Host, store *floworkdb.Store, raw []byte, approveCaps bool) pluginInstallResult {
	bad := func(code int, b map[string]any) pluginInstallResult { return pluginInstallResult{code, b} }

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return bad(http.StatusBadRequest, map[string]any{"error": "not a valid zip: " + err.Error()})
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
		return bad(http.StatusBadRequest, map[string]any{"error": "plugin.json ga ketemu di pack"})
	}
	var man pluginManifest
	if err := json.Unmarshal(manRaw, &man); err != nil {
		return bad(http.StatusBadRequest, map[string]any{"error": "plugin.json parse: " + err.Error()})
	}
	if msg := man.validate(); msg != "" {
		return bad(http.StatusBadRequest, map[string]any{"error": "manifest invalid: " + msg})
	}

	// 2) CAPS CONSENT (Phase 4.1)
	danger := scanPackCaps(zr)
	if len(danger) > 0 && !approveCaps {
		return bad(http.StatusForbidden, map[string]any{
			"error":            "pack minta caps BERBAHAYA — butuh persetujuan owner",
			"dangerous_caps":   danger,
			"approve_hint":     "install ulang dengan query ?approve_caps=1 kalau lo percaya pack ini",
			"plugin":           man.ID,
			"consent_required": true,
		})
	}

	// 3) extract ke STAGING → ATOMIC RENAME ke AgentsDir/<id>.fwagent → hot-load bersih
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
			return bad(http.StatusInternalServerError, map[string]any{"error": "mkdir: " + e.Error()})
		}
		rc, e := f.Open()
		if e != nil {
			continue
		}
		out, e := os.Create(dest)
		if e != nil {
			rc.Close()
			return bad(http.StatusInternalServerError, map[string]any{"error": "create: " + e.Error()})
		}
		_, _ = io.Copy(out, io.LimitReader(rc, 64<<20))
		out.Close()
		rc.Close()
		installed[aid]++
	}
	for aid := range installed {
		finalDir := filepath.Join(agentsRoot, aid+".fwagent")
		_ = os.RemoveAll(finalDir)
		if e := os.Rename(filepath.Join(staging, aid), finalDir); e != nil {
			return bad(http.StatusInternalServerError, map[string]any{"error": "install agent " + aid + ": " + e.Error()})
		}
	}

	// 4) daftarin kategori + crew
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
		return bad(http.StatusInternalServerError, map[string]any{"error": "upsert category: " + err.Error()})
	}
	if err := store.SetCrew(cat.ID, workers); err != nil {
		return bad(http.StatusInternalServerError, map[string]any{"error": "set crew: " + err.Error()})
	}

	// 5) SMOKE-TEST: synth ga load (pack broken) → DISABLE kategori.
	smoke := smokeTestSynth(host, synth)
	if smoke == "not_loaded" {
		cat.Enabled = false
		_ = store.UpsertCategory(cat)
	}

	return pluginInstallResult{0, map[string]any{
		"ok":             smoke != "not_loaded",
		"plugin":         man.ID,
		"category":       cat.ID,
		"enabled":        smoke != "not_loaded",
		"synth":          synth,
		"workers":        len(workers),
		"agents_extract": installed,
		"caps_approved":  len(danger) > 0 && approveCaps,
		"dangerous_caps": danger,
		"smoke":          smoke,
		"next":           "kategori LIVE — mr-flow classifier auto-discover (cache <=60s).",
	}}
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
		res := installPluginPack(host, store, raw, r.URL.Query().Get("approve_caps") == "1")
		tfWriteJSON(w, res.status, res.body)
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
