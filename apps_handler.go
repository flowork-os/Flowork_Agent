// apps_handler.go — HTTP untuk ROADMAP 4 (Apps). List app + invoke operasi (dari GUI) +
// state version (sinkron) + sajian aset GUI (untuk iframe ter-sandbox). Semua owner-session.
// invokeOp = SATU pintu yang sama dgn tool agent (manusia & AI lewat sini).
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"flowork-gui/internal/apps"
)

var appPathIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)

// GET /api/apps — daftar app (untuk launcher/grid).
func appsListHandler(mgr *apps.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		out := []map[string]any{}
		for _, a := range mgr.List() {
			out = append(out, map[string]any{
				"id": a.ID, "name": a.Name, "description": a.Description, "icon": a.Icon,
				"version": a.Version, "runtime": a.Runtime, "gui_entry": a.GUIEntry,
				"operations": a.Operations,
			})
		}
		tfWriteJSON(w, 0, map[string]any{"apps": out, "count": len(out)})
	}
}

// POST /api/apps/op {app, op, args} — invoke operasi dari GUI manusia (caller=human-gui).
func appsOpHandler(mgr *apps.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var b struct {
			App  string         `json:"app"`
			Op   string         `json:"op"`
			Args map[string]any `json:"args"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&b); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
			return
		}
		if !appPathIDRe.MatchString(b.App) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "app id invalid"})
			return
		}
		res, err := mgr.InvokeOp(b.App, b.Op, b.Args, "human-gui")
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "result": res, "version": mgr.StateVersion(b.App)})
	}
}

// POST /api/apps/install (multipart "file" = .fwpack) — install/hot-reload app TANPA restart.
// Consent exec: core app jalanin perintah OS → wajib ?approve_exec=1 (owner buka gerbang).
func appsInstallHandler() http.HandlerFunc {
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
		body, status := apps.InstallAppPack(raw, r.URL.Query().Get("approve_exec") == "1")
		tfWriteJSON(w, status, body)
	}
}

// POST /api/apps/uninstall?id= — copot app (stop core + unregister tool + hapus folder).
func appsUninstallHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !appPathIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		if err := apps.UninstallApp(id); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "uninstalled": id})
	}
}

// POST /api/apps/stop?id= — stop the app's core process (its GUI tab was closed).
// Lazy: a later op respawns it. Keeps "app runs only while a tab is open".
func appsStopHandler(mgr *apps.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !appPathIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		mgr.Stop(id)
		tfWriteJSON(w, 0, map[string]any{"ok": true, "stopped": id})
	}
}

// GET /api/apps/state?id= — versi state (GUI poll → sinkron dgn aksi agent).
func appsStateHandler(mgr *apps.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !appPathIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"version": mgr.StateVersion(id)})
	}
}

// GET /api/apps/<id>/ui/* — sajikan aset GUI app (dimuat di iframe ter-sandbox). Anti-traversal.
func appsUIHandler(mgr *apps.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/apps/")
		i := strings.IndexByte(rest, '/')
		if i < 0 {
			http.NotFound(w, r)
			return
		}
		id := rest[:i]
		rel := strings.TrimPrefix(rest[i:], "/") // "ui/index.html"
		if !appPathIDRe.MatchString(id) || !strings.HasPrefix(rel, "ui/") {
			http.NotFound(w, r)
			return
		}
		app, ok := mgr.Get(id)
		if !ok {
			http.NotFound(w, r)
			return
		}
		// resolve + containment (anti ../)
		base := filepath.Clean(app.Dir)
		full := filepath.Clean(filepath.Join(base, filepath.FromSlash(rel)))
		if c, err := filepath.Rel(base, full); err != nil || strings.HasPrefix(c, "..") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, full)
	}
}
