// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/flowork-os/flowork_Router/internal/i18n"
	"github.com/flowork-os/flowork_Router/internal/store"
)

func localeCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "en"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tag":           tag,
		"availableTags": i18n.AvailableTags(),
		"strings":       i18n.Catalog(tag),
	})
}

func localeHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		p, err := store.LoadLocalePref(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodPut, http.MethodPatch:
		var p store.LocalePref
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.SaveLocalePref(d, &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func initHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d, _ := store.Open()
	settings, _ := store.LoadSettings(d)
	if settings != nil {
		settings.Password = ""
	}
	locale, _ := store.LoadLocalePref(d)
	providers, _ := store.ListProviders(d)
	tunnel, _ := store.LoadTunnelState(d)
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       version,
		"runtime":       runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"settings":      settings,
		"locale":        locale,
		"providerCount": len(providers),
		"tunnel":        tunnel,
		"startedAt":     processStartedAt.Format(time.RFC3339),
		"now":           time.Now().UTC().Format(time.RFC3339),
	})
}

func shutdownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isLoopbackHostPort(r.RemoteAddr) {
		http.Error(w, "shutdown allowed from loopback only", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shuttingDown": true})
	go func() {
		time.Sleep(200 * time.Millisecond)
		if shutdownTriggerCh != nil {
			shutdownTriggerCh <- struct{}{}
		}
	}()
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    version,
		"runtime":    runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"startedAt":  processStartedAt.Format(time.RFC3339),
		"uptimeSec":  int64(time.Since(processStartedAt).Seconds()),
		"updateChan": "stable",
	})
}

func versionUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "noop",
		"message": "flow_router updates via rebuild — run start.sh / go build manually",
		"phase":   "phase2_pending",
	})
}

func versionShutdownHandler(w http.ResponseWriter, r *http.Request) {
	shutdownHandler(w, r)
}
