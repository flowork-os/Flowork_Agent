// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI (multi-tab): peta dok di lock/gui/README.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func maskProviderSecret(p store.ProviderConnection) store.ProviderConnection {
	if p.Data == nil {
		return p
	}
	k, ok := p.Data[store.CfgAPIKey].(string)
	if !ok || k == "" {
		return p
	}
	cp := make(map[string]any, len(p.Data))
	for kk, vv := range p.Data {
		cp[kk] = vv
	}
	if len(k) <= 8 {
		cp[store.CfgAPIKey] = strings.Repeat("•", len(k))
	} else {
		cp[store.CfgAPIKey] = k[:4] + strings.Repeat("•", len(k)-8) + k[len(k)-4:]
	}
	p.Data = cp
	return p
}

func providersListAddHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		providers, err := store.ListProviders(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range providers {
			providers[i] = maskProviderSecret(providers[i])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": providers})
	case http.MethodPost:
		var p store.ProviderConnection
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpsertProvider(d, &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func providerCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	rest := r.URL.Path[len("/api/providers/"):]
	if rest == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if i := indexByte(rest, '/'); i >= 0 {
		id, action := rest[:i], rest[i+1:]
		providerSubActionHandler(w, r, id, action)
		return
	}
	id := rest
	switch r.Method {
	case http.MethodGet:
		p, err := store.GetProvider(d, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		masked := maskProviderSecret(*p)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(masked)
	case http.MethodPut:
		var p store.ProviderConnection
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		p.ID = id
		if err := store.UpsertProvider(d, &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	case http.MethodDelete:
		if err := store.DeleteProvider(d, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func presetsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": store.Presets})
}

func combosListAddHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListCombos(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var c store.Combo
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpsertCombo(d, &c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(c)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func comboCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	id := r.URL.Path[len("/api/combos/"):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var c store.Combo
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		c.ID = id
		if err := store.UpsertCombo(d, &c); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c)
	case http.MethodDelete:
		if err := store.DeleteCombo(d, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func apiKeysListAddHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListAPIKeys(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var body struct {
			Name             string  `json:"name"`
			AllowedProviders string  `json:"allowedProviders"`
			DailyCapUsd      float64 `json:"dailyCapUsd"`
			MonthlyCapUsd    float64 `json:"monthlyCapUsd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.Name == "" {
			body.Name = "key-" + fmt.Sprintf("%d", os.Getpid())
		}
		k, err := store.GenerateAPIKey(d, body.Name, body.AllowedProviders, body.DailyCapUsd, body.MonthlyCapUsd)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(k)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func apiKeyCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	id := r.URL.Path[len("/api/keys/"):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed (POST /api/keys to create, DELETE to revoke)", http.StatusMethodNotAllowed)
		return
	}
	if err := store.DeleteAPIKey(d, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func proxyPoolsListAddHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		items, err := store.ListProxyPools(d)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var p store.ProxyPool
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpsertProxyPool(d, &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func proxyPoolCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	rest := r.URL.Path[len("/api/proxy-pools/"):]
	if rest == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if i := indexByte(rest, '/'); i >= 0 {
		id, action := rest[:i], rest[i+1:]
		if action == "test" {
			proxyPoolTestHandler(w, r, id)
			return
		}
		http.Error(w, "unknown proxy-pool sub-action: "+action, http.StatusNotFound)
		return
	}
	id := rest
	switch r.Method {
	case http.MethodPut:
		var p store.ProxyPool
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		p.ID = id
		if err := store.UpsertProxyPool(d, &p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(p)
	case http.MethodDelete:
		if err := store.DeleteProxyPool(d, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func mediaProvidersHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	switch r.Method {
	case http.MethodGet:
		cat := r.URL.Query().Get("category")
		items, err := store.ListMediaProviders(d, cat)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items, "count": len(items)})
	case http.MethodPost:
		var m store.MediaProvider
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.UpsertMediaProvider(d, &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func mediaProviderCRUDHandler(w http.ResponseWriter, r *http.Request) {
	d, _ := store.Open()
	id := r.URL.Path[len("/api/media-providers/"):]
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	cat := r.URL.Query().Get("category")
	switch r.Method {
	case http.MethodPut:
		var m store.MediaProvider
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "parse: "+err.Error(), http.StatusBadRequest)
			return
		}
		m.ID = id
		if m.Category == "" {
			m.Category = cat
		}
		if err := store.UpsertMediaProvider(d, &m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(m)
	case http.MethodDelete:
		if cat == "" {
			http.Error(w, "category query param required for delete", http.StatusBadRequest)
			return
		}
		if err := store.DeleteMediaProvider(d, cat, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
