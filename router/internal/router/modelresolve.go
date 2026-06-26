// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"database/sql"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func resolveModel(d *sql.DB, model string) (string, string) {
	if aliases, err := store.ListModelAliases(d); err == nil {
		for _, a := range aliases {
			if a.Alias == model {
				return a.Model, a.ProviderID
			}
		}
	}
	if customs, err := store.ListCustomModels(d); err == nil {
		for _, c := range customs {
			if c.Model == model {
				return model, c.ProviderID
			}
		}
	}
	return model, ""
}

func pinProvider(d *sql.DB, matches []store.ProviderConnection, providerID string) []store.ProviderConnection {
	if providerID == "" {
		return matches
	}
	for _, p := range matches {
		if p.ID == providerID {
			return []store.ProviderConnection{p}
		}
	}
	if p, _ := store.GetProvider(d, providerID); p != nil && p.IsActive {
		return []store.ProviderConnection{*p}
	}
	return nil
}

func filterDisabled(d *sql.DB, matches []store.ProviderConnection, model string) []store.ProviderConnection {
	var out []store.ProviderConnection
	for _, p := range matches {
		if store.IsModelDisabled(d, p.Provider, model) || store.IsModelDisabled(d, p.ID, model) {
			continue
		}
		out = append(out, p)
	}
	return out
}
