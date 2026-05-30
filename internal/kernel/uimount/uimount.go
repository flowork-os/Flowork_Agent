// === LOCKED FILE ===
// Status: STABLE (RESERVED for future) — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Plugin UI mount package — currently NOT imported by main.go.
//   Reserved for Phase 11+ multi-plugin GUI shell. Audit pass:
//   - Path traversal guard di ServeAsset (filepath.Rel + ".." check)
//   - Cache-Control no-store
//   - I18n locale fallback to "en"
//   - Minor: Index handler line 113 pakai dummy Request — kalau di-wire,
//     ganti ke ServeIndex yang pakai real r.
//
// Package uimount — kumpulkan kontribusi UI dari plugin discovery,
// expose handler HTTP untuk:
//
//   - GET /                         primary gui shell (gui-shell-id/ui/index.html)
//   - GET /ui/<plugin-id>/<file>    static asset dari plugin folder ui/
//   - GET /api/contributions        merged tab list dari semua plugin
//   - GET /api/i18n?locale=en       merged dictionary dari semua plugin
//
// Kernel HTTP server mux ini di-mount oleh main.go.

package uimount

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/kernel/loader"
)

// Registry — snapshot kontribusi UI yang sudah di-resolve dari manifest
// + folder fisik. Read-only setelah Build.
type Registry struct {
	Tabs        []TabContribution
	I18nByKey   map[string]map[string]string // key -> locale -> text
	PrimaryRoot string                        // absolute path ke gui-shell ui/index.html
	pluginRoots map[string]string             // plugin_id -> absolute path ke folder plugin
}

// TabContribution — satu entry tab yang kernel exposed ke gui-shell.
type TabContribution struct {
	PluginID   string `json:"plugin_id"`
	Label      string `json:"label"`
	Icon       string `json:"icon,omitempty"`
	Path       string `json:"path,omitempty"`   // URL relatif yang gui-shell load di iframe
	LabelKey   string `json:"label_key,omitempty"`
	ParentHub  string `json:"parent_hub,omitempty"`
}

// Build merge kontribusi UI dari setiap discovery yang state-nya bukan
// failed. PrimaryRoot dipilih dari plugin kind=gui pertama yang punya
// ui_contributes.tab.path; kalau gak ada, return error.
func Build(discoveries []loader.Discovery) (*Registry, error) {
	reg := &Registry{
		I18nByKey:   map[string]map[string]string{},
		pluginRoots: map[string]string{},
	}

	for _, d := range discoveries {
		if d.Manifest == nil {
			continue
		}
		reg.pluginRoots[d.Manifest.ID] = d.Path

		// Merge i18n keys ke dict global.
		for k, v := range d.Manifest.I18nKeys {
			if reg.I18nByKey[k] == nil {
				reg.I18nByKey[k] = map[string]string{}
			}
			for locale, text := range v {
				reg.I18nByKey[k][locale] = text
			}
		}

		// Resolve ui contribution kalau ada.
		if d.Manifest.UIContributes != nil && d.Manifest.UIContributes.Tab != nil {
			t := d.Manifest.UIContributes.Tab
			contribPath := ""
			if t.Path != "" {
				contribPath = "/ui/" + d.Manifest.ID + "/" + filepath.ToSlash(t.Path[len("ui/"):])
				if !strings.HasPrefix(t.Path, "ui/") {
					contribPath = "/ui/" + d.Manifest.ID + "/" + filepath.ToSlash(t.Path)
				}
			}
			reg.Tabs = append(reg.Tabs, TabContribution{
				PluginID:  d.Manifest.ID,
				Label:     d.Manifest.DisplayName,
				Icon:      t.Icon,
				Path:      contribPath,
				LabelKey:  t.LabelKey,
				ParentHub: t.ParentHub,
			})

			// Primary shell = first kind=gui plugin yang sumbang index.html.
			if d.Manifest.Kind == loader.KindGUI && reg.PrimaryRoot == "" {
				abs := filepath.Join(d.Path, filepath.FromSlash(t.Path))
				if _, err := os.Stat(abs); err == nil {
					reg.PrimaryRoot = abs
				}
			}
		}
	}

	if reg.PrimaryRoot == "" {
		return reg, errors.New("no kind=gui plugin with ui_contributes.tab.path found")
	}
	return reg, nil
}

// HandlerSet returns http handlers untuk mount oleh kernel main.go.
func (r *Registry) HandlerSet() Handlers {
	return Handlers{reg: r}
}

type Handlers struct {
	reg *Registry
}

// Index serves primary gui shell index.html.
func (h Handlers) Index(w http.ResponseWriter, _ *http.Request) {
	http.ServeFile(w, &http.Request{Method: "GET"}, h.reg.PrimaryRoot)
}

// IndexHandler — alternative yang use http.ServeFile dengan benar.
func (h Handlers) ServeIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, h.reg.PrimaryRoot)
}

// ServeAsset handle path /ui/<plugin-id>/<sub-path>.
func (h Handlers) ServeAsset(w http.ResponseWriter, r *http.Request) {
	// Path = "/ui/<plugin-id>/<rest>"
	const prefix = "/ui/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	rest := r.URL.Path[len(prefix):]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		http.NotFound(w, r)
		return
	}
	pluginID := rest[:slash]
	subPath := rest[slash+1:]
	root, ok := h.reg.pluginRoots[pluginID]
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Filesystem layout di plugin folder: ui/<files>. Jadi prepend ui/.
	fileAbs := filepath.Join(root, "ui", filepath.FromSlash(subPath))
	// Guard: tidak boleh escape root via ../
	rel, err := filepath.Rel(filepath.Join(root, "ui"), fileAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(fileAbs); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, fileAbs)
}

// Contributions serves /api/contributions — JSON yang gui-shell pakai
// untuk render sidebar.
func (h Handlers) Contributions(w http.ResponseWriter, _ *http.Request) {
	out := struct {
		Tabs  []TabContribution `json:"tabs"`
		Count int               `json:"count"`
	}{Tabs: h.reg.Tabs, Count: len(h.reg.Tabs)}
	writeJSON(w, out)
}

// I18n serves /api/i18n?locale=en — dict (key -> text) sudah resolved
// untuk satu locale. Fallback en kalau locale gak ada.
func (h Handlers) I18n(w http.ResponseWriter, r *http.Request) {
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = "en"
	}
	out := map[string]string{}
	for key, byLocale := range h.reg.I18nByKey {
		if v, ok := byLocale[locale]; ok {
			out[key] = v
		} else if v, ok := byLocale["en"]; ok {
			out[key] = v
		}
	}
	writeJSON(w, out)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
