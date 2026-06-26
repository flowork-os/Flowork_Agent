// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"flowork-gui/internal/agentdb"
	fwapps "flowork-gui/internal/apps"
	"flowork-gui/internal/kernelhost"
)

func appGrantsHandler(mgr *fwapps.Manager, host *kernelhost.Host) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !appPathIDRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid id"})
			return
		}
		store, err := host.OpenAgentStore(id)
		if err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		defer store.Close()

		switch r.Method {
		case http.MethodGet:
			granted := map[string]bool{}
			if g, gerr := store.ListAppGrants(); gerr == nil {
				for _, a := range g {
					granted[a] = true
				}
			}

			out := []map[string]any{}
			for _, a := range mgr.List() {

				connectors := 0
				for _, o := range a.Operations {
					if o.Tool {
						connectors++
					}
				}
				out = append(out, map[string]any{
					"id": a.ID, "name": a.Name, "permitted": granted[a.ID],
					"connectors": connectors,
				})
			}
			tfWriteJSON(w, 0, map[string]any{"apps": out, "count": len(out)})

		case http.MethodPost:
			var body struct {
				AppID string `json:"app_id"`
				Allow bool   `json:"allow"`
			}
			if derr := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); derr != nil {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
				return
			}
			appID := strings.TrimSpace(body.AppID)
			if !appPathIDRe.MatchString(appID) {
				tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid app_id"})
				return
			}
			var serr error
			if body.Allow {
				serr = store.GrantApp(appID)
			} else {
				serr = store.RevokeApp(appID)
			}
			if serr != nil {
				tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": serr.Error()})
				return
			}
			applyAppCaps(host, id, store)
			tfWriteJSON(w, 0, map[string]any{"ok": true, "app_id": appID, "permitted": body.Allow})

		default:
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET/POST only"})
		}
	}
}

func applyAppCaps(host *kernelhost.Host, agentID string, store *agentdb.Store) {
	if host == nil || host.Broker == nil || store == nil {
		return
	}
	grants, err := store.ListAppGrants()
	if err != nil {
		return
	}
	final := []string{}
	for _, c := range host.Broker.Approved(agentID) {
		if strings.HasPrefix(c, "app:") {
			continue
		}
		final = append(final, c)
	}
	for _, a := range grants {
		final = append(final, "app:"+a)
	}
	host.Broker.Approve(agentID, final)
}
