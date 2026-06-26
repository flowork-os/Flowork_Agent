// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package i18n

import (
	"embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	loadOnce sync.Once
	catalog  = map[string]map[string]string{}
)

func loadCatalog() {
	loadOnce.Do(func() {
		entries, err := localeFS.ReadDir("locales")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := localeFS.ReadFile("locales/" + e.Name())
			if err != nil {
				continue
			}
			var m map[string]string
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			tag := strings.TrimSuffix(e.Name(), ".json")
			catalog[tag] = m
		}
	})
}

func AvailableTags() []string {
	loadCatalog()
	out := make([]string, 0, len(catalog))
	for k := range catalog {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func Catalog(tag string) map[string]string {
	loadCatalog()
	if tag == "" {
		tag = "en"
	}
	src := catalog[tag]
	if src == nil {
		src = catalog["en"]
	}
	if src == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func T(tag, key string) string {
	loadCatalog()
	if tag != "" {
		if v := catalog[tag][key]; v != "" {
			return v
		}
	}
	if v := catalog["en"][key]; v != "" {
		return v
	}
	return key
}
