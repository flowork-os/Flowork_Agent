// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/threat-radar.md

package scanapi

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/scanner"
)

func nucleiTemplatesDir() string {
	if v := strings.TrimSpace(os.Getenv("NUCLEI_TEMPLATES_DIR")); v != "" {
		if st, err := os.Stat(v); err == nil && st.IsDir() {
			return v
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, c := range []string{
		filepath.Join(home, "nuclei-templates"),
		filepath.Join(home, ".local", "nuclei-templates"),
		filepath.Join(home, ".config", "nuclei", "templates"),
	} {
		if st, e := os.Stat(c); e == nil && st.IsDir() {
			return c
		}
	}
	return ""
}

type nucleiPack struct {
	ID    string
	Name  string
	Count int
}

var (
	nucleiPackMu    sync.Mutex
	nucleiPackCache []nucleiPack
	nucleiPackReady bool
)

func resetNucleiPackCache() {
	nucleiPackMu.Lock()
	nucleiPackCache, nucleiPackReady = nil, false
	nucleiPackMu.Unlock()
}

func enumerateNucleiPacks() []nucleiPack {
	nucleiPackMu.Lock()
	defer nucleiPackMu.Unlock()
	if nucleiPackReady {
		return nucleiPackCache
	}
	dir := nucleiTemplatesDir()
	if dir == "" {
		nucleiPackReady = true
		return nucleiPackCache
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		nucleiPackReady = true
		return nucleiPackCache
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		n := 0
		_ = filepath.WalkDir(sub, func(_ string, d os.DirEntry, werr error) error {
			if werr == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
				n++
			}
			return nil
		})
		if n > 0 {
			nucleiPackCache = append(nucleiPackCache, nucleiPack{ID: "nuclei:" + e.Name(), Name: e.Name(), Count: n})
		}
	}
	sort.Slice(nucleiPackCache, func(i, j int) bool { return nucleiPackCache[i].Count > nucleiPackCache[j].Count })
	nucleiPackReady = true
	return nucleiPackCache
}

func nucleiExclusionArgs(disabled map[string]bool, dir string) []string {
	if dir == "" || len(disabled) == 0 {
		return nil
	}
	ids := make([]string, 0, len(disabled))
	for id := range disabled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []string
	for _, id := range ids {
		name, ok := strings.CutPrefix(id, "nuclei:")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, "-exclude-templates", filepath.Join(dir, name))
	}
	return out
}

func applyNucleiExclusions(store *floworkdb.Store, binary string, args []string) []string {
	if strings.ToLower(strings.TrimSuffix(filepath.Base(binary), ".exe")) != "nuclei" {
		return args
	}
	disabled, err := store.ListScannerDisabled()
	if err != nil {
		return args
	}
	excl := nucleiExclusionArgs(disabled, nucleiTemplatesDir())
	if len(excl) == 0 {
		return args
	}
	return append(append([]string{}, args...), excl...)
}

type registryItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Count     int    `json:"count"`
	Installed bool   `json:"installed"`
	Core      bool   `json:"core"`
}

type registryPlane struct {
	Key       string         `json:"key"`
	Label     string         `json:"label"`
	Removable bool           `json:"removable"`
	Items     []registryItem `json:"items"`
}

func ScannerRegistryHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		disabled, err := store.ListScannerDisabled()
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		totalInstalled := 0

		audItems := make([]registryItem, 0, 128)
		for _, n := range scanner.Names() {
			audItems = append(audItems, registryItem{ID: "auditor:" + n, Name: n, Count: 1, Installed: true, Core: true})
			totalInstalled++
		}
		toolItems := []registryItem{}
		for _, n := range scanner.ToolNames() {
			toolItems = append(toolItems, registryItem{ID: "tool:" + n, Name: n, Count: 1, Installed: true, Core: true})
			totalInstalled++
		}
		nucItems := []registryItem{}
		for _, p := range enumerateNucleiPacks() {
			inst := !disabled[p.ID]
			nucItems = append(nucItems, registryItem{ID: p.ID, Name: p.Name, Count: p.Count, Installed: inst, Core: false})
			if inst {
				totalInstalled += p.Count
			}
		}

		planes := []registryPlane{
			{Key: "auditor", Label: "Code Auditors — defensive core", Removable: false, Items: audItems},
			{Key: "tool", Label: "Dependency / Secret / IaC tools", Removable: false, Items: toolItems},
			{Key: "nuclei", Label: "Nuclei Templates — offensive (target checks)", Removable: true, Items: nucItems},
		}
		tfWriteJSON(w, 0, map[string]any{"planes": planes, "total_installed": totalInstalled})
	}
}

func ScannerRegistryToggleHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			ID        string `json:"id"`
			Installed bool   `json:"installed"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body); err != nil || strings.TrimSpace(body.ID) == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
			return
		}
		if !strings.HasPrefix(body.ID, "nuclei:") {
			tfWriteJSON(w, http.StatusForbidden, map[string]any{"error": "scanner core defensif ga bisa di-uninstall"})
			return
		}
		if err := store.SetScannerInstalled(body.ID, body.Installed); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "id": body.ID, "installed": body.Installed})
	}
}
