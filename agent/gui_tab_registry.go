// gui_tab_registry.go — SWITCH GUI (papan colokan tab, POLA-A). BEKU, default KOSONG = aman.
// Owner 2026-07-03: sebelum bekuin SEMUA GUI, pasang stop-kontak biar nambah tab ga bobok tembok beku.
//
// Tab GUI baru dicolok lewat SIBLING deletable `gui_tab_<x>_ext.go` (`func init(){ RegisterGUITab(...) }`)
// → `app.js` (BEKU) fetch `/api/gui/tabs-ext` pas boot → APPEND ke nav + ACTIVE_TABS + muat i18n domain-nya.
// Builtin nav (index.html BEKU) + app.js builtin NOL disentuh. Hapus sibling → tab ilang, GUI tetep jalan.
// 📄 Dok: FLowork_os/lock/gui-tab-registry.md
package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
)

// GUITabSpec — metadata 1 tab GUI ekstensi (di-serve ke app.js).
type GUITabSpec struct {
	Name       string `json:"name"`        // id tab → app.js muat /tabs/<name>.js (deletable)
	Icon       string `json:"icon"`        // emoji di nav (opsional)
	Label      string `json:"label"`       // label polos (fallback kalau ga ada i18n)
	LabelKey   string `json:"label_key"`   // i18n key (opsional, menang atas Label)
	I18nDomain string `json:"i18n_domain"` // domain i18n ekstra buat dimuat (opsional)
	Order      int    `json:"order"`       // urutan di nav
}

var (
	guiTabMu       sync.Mutex
	guiTabRegistry []GUITabSpec
)

// RegisterGUITab — colok tab GUI ekstensi. Dipanggil dari init() sibling gui_tab_<x>_ext.go.
// Idempoten (dedup by name), thread-safe. Nama kosong = diabaikan.
func RegisterGUITab(s GUITabSpec) {
	if s.Name == "" {
		return
	}
	guiTabMu.Lock()
	defer guiTabMu.Unlock()
	for _, e := range guiTabRegistry {
		if e.Name == s.Name {
			return
		}
	}
	guiTabRegistry = append(guiTabRegistry, s)
}

// guiTabsExtList — salinan terurut registry (dipanggil handler; default kosong = nol tab ekstensi).
func guiTabsExtList() []GUITabSpec {
	guiTabMu.Lock()
	out := make([]GUITabSpec, len(guiTabRegistry))
	copy(out, guiTabRegistry)
	guiTabMu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

func init() {
	RegisterFeature(Feature{Name: "gui-tab-registry", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/gui/tabs-ext", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"tabs": guiTabsExtList()})
		})
	}})
}
