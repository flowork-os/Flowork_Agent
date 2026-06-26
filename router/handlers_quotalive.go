// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"net/http"
	"time"

	"github.com/flowork-os/flowork_Router/internal/creds"
	"github.com/flowork-os/flowork_Router/internal/quotalive"
)

func quotaLiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":     "provider query param required",
			"supported": quotalive.List(),
		})
		return
	}
	impl := quotalive.Get(provider)
	if impl == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error":     "no live fetcher registered for " + provider,
			"supported": quotalive.List(),
		})
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		var err error
		token, err = resolveLiveToken(provider)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":    err.Error(),
				"hint":     "pass ?token=<value> or configure the provider's credentials",
				"provider": provider,
			})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	snap, err := impl.Fetch(ctx, quotalive.Params{Token: token})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":    err.Error(),
			"provider": provider,
		})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func resolveLiveToken(provider string) (string, error) {
	switch provider {
	case "claude":

		c, err := creds.LoadValid()
		if err != nil {
			return "", err
		}
		return c.ClaudeAiOauth.AccessToken, nil
	default:
		return "", errNoAutoToken(provider)
	}
}

type tokenErr struct{ msg string }

func (e tokenErr) Error() string { return e.msg }

func errNoAutoToken(p string) error {
	return tokenErr{msg: "no auto-token loader for " + p + " in this build; pass ?token=<value>"}
}
