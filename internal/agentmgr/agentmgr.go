// Package agentmgr — HTTP handlers untuk manage AI agent.
//
// Bekerja langsung di atas ~/.flowork/agents/<id>.fwagent/ tanpa proxy ke
// kernel terpisah; kernel sekarang embedded (lihat internal/kernelhost).
// Hot-reload watcher di kernelhost yang pickup perubahan disk → tidak
// perlu "restart kernel" endpoint lagi.
//
// Endpoint:
//
//	POST   /api/agents/upload          .fwagent.zip → extract ke staging
//	GET    /api/agents/download?id=    bundle balik jadi .fwagent.zip
//	DELETE /api/agents/remove?id=      hapus folder agent
//	GET    /api/agents/config?id=      baca config.json
//	POST   /api/agents/config?id=      tulis config.json (router/prompt/tools/schedule)
//
// List agent + RPC pakai handler kernelhost langsung di /api/kernel/*.

package agentmgr

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/kernel/loader"
)

// reID — sinkron sama loader.manifest.go reID.
var reID = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

// Reload — kernelhost daftarkan callback di main.go. Dipanggil setelah
// config save / db reset supaya live instance pickup config baru (Unload
// + LoadInstance + AutoBoot daemon → env baru kebawa).
var Reload func(agentID string) error

// agentFolder return absolute path ke folder agent.
func agentFolder(id string) string {
	return filepath.Join(loader.AgentsDir(), id+".fwagent")
}

// UploadHandler — POST /api/agents/upload. multipart `file` berisi
// .fwagent.zip. Manifest minimal divalidasi (id, kind, entry) sebelum
// extract.
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "parse form: " + err.Error()})
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "missing file field"})
		return
	}
	defer file.Close()

	lower := strings.ToLower(hdr.Filename)
	if !strings.HasSuffix(lower, ".zip") {
		httpx.WriteJSON(w, map[string]any{"error": "expected .fwagent.zip filename"})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "read upload: " + err.Error()})
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "not a valid zip: " + err.Error()})
		return
	}

	manifestEntry, rootPrefix, err := findManifest(zr)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	manifestBody, err := readZipEntry(manifestEntry)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "read manifest: " + err.Error()})
		return
	}
	var manifest struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Entry string `json:"entry"`
	}
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "manifest parse: " + err.Error()})
		return
	}
	if !reID.MatchString(manifest.ID) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid manifest.id"})
		return
	}

	targetDir := agentFolder(manifest.ID)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "mkdir target: " + err.Error()})
		return
	}
	written := 0
	for _, f := range zr.File {
		rel := f.Name
		if rootPrefix != "" {
			if !strings.HasPrefix(rel, rootPrefix) {
				continue
			}
			rel = strings.TrimPrefix(rel, rootPrefix)
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		dest := filepath.Join(targetDir, filepath.FromSlash(rel))
		clean, err := filepath.Rel(targetDir, dest)
		if err != nil || strings.HasPrefix(clean, "..") {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "mkdir: " + err.Error()})
			return
		}
		if err := extractFile(f, dest); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "extract " + rel + ": " + err.Error()})
			return
		}
		written++
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":            true,
		"agent_id":      manifest.ID,
		"kind":          manifest.Kind,
		"entry":         manifest.Entry,
		"files_written": written,
		"target_dir":    targetDir,
		"next":          "kernel hot-reload pickup otomatis",
	})
}

// DownloadHandler — GET /api/agents/download?id=<id>.
//
// Bundle SELURUH folder agent jadi .fwagent.zip — termasuk:
//   - manifest.json
//   - agent.wasm (kalau di-stage) atau main.go (source)
//   - workspace/ + state.db (SQLite per-agent berisi semua setting)
//   - ui/, i18n/, sub-folder lain
//
// Authoritative source: `<project>/agents/<id>/` kalau ada (preserve
// source code + state lengkap). Fallback ke staged
// `~/.flowork/agents/<id>.fwagent/` (untuk agent yang di-install dari
// zip tanpa source).
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	// Resolve source dulu (authoritative + state lengkap).
	srcDir := ""
	if cwd, err := os.Getwd(); err == nil {
		cand := filepath.Join(cwd, "agents", id)
		if stat, err := os.Stat(cand); err == nil && stat.IsDir() {
			srcDir = cand
		}
	}
	if srcDir == "" {
		srcDir = agentFolder(id) // fallback ke staged
	}
	if _, err := os.Stat(srcDir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found: " + id})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.fwagent.zip"`, id))
	w.Header().Set("Cache-Control", "no-store")

	zw := zip.NewWriter(w)
	defer zw.Close()

	_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return nil
		}
		// Skip artifact build (.git, node_modules, dst). Workspace +
		// state.db sengaja diikutin biar agent fully portable.
		base := filepath.Base(rel)
		if base == ".git" || base == "node_modules" || base == ".reload-trigger" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		zipPath := filepath.ToSlash(rel)
		if info.IsDir() {
			// Bikin entry dir biar zip preserve folder kosong (mis. workspace/cache/).
			_, _ = zw.Create(zipPath + "/")
			return nil
		}
		f, err := zw.Create(zipPath)
		if err != nil {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer src.Close()
		_, _ = io.Copy(f, src)
		return nil
	})
}

// ToggleHandler — POST /api/agents/toggle?id=<id>&disabled=<0|1>. Flip
// enable/disable di DB (meta.disabled) lalu reload agent. Switch off →
// kernel unload + daemon stop. Switch on → instantiate + auto-boot.
func ToggleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dir := agentFolder(id)
	if _, err := os.Stat(dir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found"})
		return
	}
	disabledFlag := strings.TrimSpace(r.URL.Query().Get("disabled"))
	disabled := disabledFlag == "1" || strings.EqualFold(disabledFlag, "true")

	dbPath := agentdb.Resolve(id, dir)
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	if err := store.SetDisabled(disabled); err != nil {
		store.Close()
		httpx.WriteJSON(w, map[string]any{"error": "set: " + err.Error()})
		return
	}
	store.Close()

	reloadErr := ""
	if Reload != nil {
		if err := Reload(id); err != nil {
			reloadErr = err.Error()
			log.Printf("toggle: reload %s: %v", id, err)
		}
	}
	httpx.WriteJSON(w, map[string]any{
		"ok":         true,
		"id":         id,
		"enabled":    !disabled,
		"reload_err": reloadErr,
	})
}

// DBResetHandler — POST /api/agents/db/reset?id=<id>. Hapus state.db
// agent → kernel hot-reload bakal touch ulang file kosong. Workspace
// folder lain (selain state.db) tetap utuh.
func DBResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			httpx.WriteJSON(w, map[string]any{"ok": true, "note": "db belum ada, ngga ada yang di-reset"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	if err := os.Remove(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "remove: " + err.Error()})
		return
	}
	// Touch ulang biar agent yang lagi jalan ngga error saat open.
	if f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o644); err == nil {
		_ = f.Close()
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "path": dbPath})
}

// InteractionsHandler — GET /api/agents/interactions?id=<id>&channel=&actor=&limit=
// List episodic interaction log dari state.db agent. Roadmap section 1.
//
// ⚠️ Endpoint ini buat dashboard / audit / manual recall — JANGAN
// di-auto-inject ke system prompt (over-prompt risk, lihat standar
// section 11). Max 500 row per call, default 50.
func InteractionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dbPath := agentdb.Resolve(id, agentFolder(id))
	if _, err := os.Stat(dbPath); err != nil {
		httpx.WriteJSON(w, map[string]any{"items": []any{}, "note": "db belum ada"})
		return
	}
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()

	channel := strings.TrimSpace(r.URL.Query().Get("channel"))
	actor := strings.TrimSpace(r.URL.Query().Get("actor"))
	limit := 50
	if s := strings.TrimSpace(r.URL.Query().Get("limit")); s != "" {
		if n, perr := strconv.Atoi(s); perr == nil {
			limit = n
		}
	}
	items, err := store.ListInteractions(channel, actor, limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "list: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
}

// RemoveHandler — DELETE /api/agents/remove?id=<id>.
func RemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dir := agentFolder(id)
	if _, err := os.Stat(dir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found"})
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "remove: " + err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "removed": id})
}

// ConfigHandler — GET/POST /api/agents/config?id=<id>.
//
// Standar agent (7 section, semua terisolasi per agent):
//
//   1. prompt      — system prompt / persona
//   2. schedule    — cron jobs ([{id, cron, task}])
//   3. tools       — capability flags ([telegram, router, kv, fs, net])
//   4. skills      — reusable behaviors ([{id, trigger, instructions}])
//   5. workspace   — host-side folder, di-mount kernel ke guest /workspace
//   6. settings    — router endpoint + model + arbitrary API credentials
//   7. database    — SQLite per-agent di workspace/state.db (file isolated)
//
// Section 5 & 7 host-side: kernel kelola di disk, ngga ada di config.json.
//
// Schema config.json:
//
//	{
//	  "prompt":   "system prompt string...",
//	  "schedule": [{"id":"morning-news","cron":"0 7 * * *","task":"..."}],
//	  "tools":    ["telegram", "kv", ...],
//	  "skills":   [{"id":"summarize","trigger":"/sum","instructions":"..."}],
//	  "router":   {"url":"...","model":"..."},
//	  "secrets":  {"TELEGRAM_BOT_TOKEN":"...","GOOGLE_API_KEY":"...", ...}
//	}
//
// File ini dibaca kernel saat boot dan di-inject ke agent via env:
//   FLOWORK_AGENT_CONFIG  — JSON utuh
//   FLOWORK_AGENT_ID      — id agent
//   FLOWORK_AGENT_WORKSPACE = /workspace
//   FLOWORK_AGENT_DB       = /workspace/state.db
//   FLOWORK_SHARED_WORKSPACE = /shared (kalau cap fs:shared)
//   <KEY>=<value>          — tiap secrets.* di-expand jadi env (telegram/google/dll)
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !reID.MatchString(id) {
		httpx.WriteJSON(w, map[string]any{"error": "invalid id"})
		return
	}
	dir := agentFolder(id)
	if _, err := os.Stat(dir); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent not found"})
		return
	}
	// Resolve & open DB. HARDCODED di `<source-or-staged>/workspace/state.db`.
	dbPath := agentdb.Resolve(id, dir)
	store, err := agentdb.Open(dbPath)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "open db: " + err.Error()})
		return
	}
	defer store.Close()
	// Migrate config.json (kalau masih ada — first GET/POST after upgrade).
	_ = store.MigrateFromJSON(dir)

	switch r.Method {
	case http.MethodGet:
		cfg, err := store.Load()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "load: " + err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"config": cfg, "exists": true, "db": dbPath})

	case http.MethodPost, http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "read body: " + err.Error()})
			return
		}
		var cfg map[string]any
		if err := json.Unmarshal(body, &cfg); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "json parse: " + err.Error()})
			return
		}
		if err := store.Save(cfg); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "save: " + err.Error()})
			return
		}
		// Trigger kernel reload: callback registered by main.go pointing
		// ke host.ReloadAgent. inotify ngga recurse ke subfolder, jadi
		// file-system trigger ngga reliable — callback langsung lebih
		// deterministik.
		reloadErr := ""
		if Reload != nil {
			if err := Reload(id); err != nil {
				reloadErr = err.Error()
				log.Printf("config: reload %s: %v", id, err)
			}
		}
		httpx.WriteJSON(w, map[string]any{"ok": true, "db": dbPath, "reload_err": reloadErr})

	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// ── zip helpers ────────────────────────────────────────────────────────────

func findManifest(zr *zip.Reader) (*zip.File, string, error) {
	for _, f := range zr.File {
		if filepath.Base(f.Name) != "manifest.json" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(f.Name))
		if dir == "." || dir == "" {
			return f, "", nil
		}
		return f, dir + "/", nil
	}
	return nil, "", errors.New("manifest.json not found in zip")
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 1<<20))
}

func extractFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
