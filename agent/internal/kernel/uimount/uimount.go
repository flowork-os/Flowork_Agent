// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

type Registry struct {
	Tabs        []TabContribution
	I18nByKey   map[string]map[string]string
	PrimaryRoot string
	pluginRoots map[string]string
}

type TabContribution struct {
	PluginID  string `json:"plugin_id"`
	Label     string `json:"label"`
	Icon      string `json:"icon,omitempty"`
	Path      string `json:"path,omitempty"`
	LabelKey  string `json:"label_key,omitempty"`
	ParentHub string `json:"parent_hub,omitempty"`
}

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

		for k, v := range d.Manifest.I18nKeys {
			if reg.I18nByKey[k] == nil {
				reg.I18nByKey[k] = map[string]string{}
			}
			for locale, text := range v {
				reg.I18nByKey[k][locale] = text
			}
		}

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

func (r *Registry) HandlerSet() Handlers {
	return Handlers{reg: r}
}

type Handlers struct {
	reg *Registry
}

func (h Handlers) Index(w http.ResponseWriter, _ *http.Request) {
	http.ServeFile(w, &http.Request{Method: "GET"}, h.reg.PrimaryRoot)
}

func (h Handlers) ServeIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, h.reg.PrimaryRoot)
}

func (h Handlers) ServeAsset(w http.ResponseWriter, r *http.Request) {

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

	fileAbs := filepath.Join(root, "ui", filepath.FromSlash(subPath))

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

func (h Handlers) Contributions(w http.ResponseWriter, _ *http.Request) {
	out := struct {
		Tabs  []TabContribution `json:"tabs"`
		Count int               `json:"count"`
	}{Tabs: h.reg.Tabs, Count: len(h.reg.Tabs)}
	writeJSON(w, out)
}

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
