// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: Proxy Pools (registry deploy plug-and-play) → dok lock/gui/Proxy Pools.md  ⚠️ FROZEN.
// Nambah target deploy: sibling proxy_<x>_ext.go + RegisterProxyDeployTarget. Cara: CARAFREEZE.MD
// (POLA A) + lock/plug-and-play.md. Pola freeze: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

func init() {
	RegisterExtraRoute(func(mux *http.ServeMux) {
		mux.HandleFunc("/api/proxy-pools/deploy-targets", proxyDeployTargetsListHandler)
		mux.HandleFunc("/api/proxy-pools/deploy/", proxyDeployDispatchHandler)
	})
}

func proxyDeployTargetsListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list := make([]map[string]any, 0, len(proxyDeployTargets))
	for _, name := range proxyDeployTargetNames() {
		t, _ := getProxyDeployTarget(name)
		e := map[string]any{"name": name}
		if t.CLIBin != "" {
			_, err := exec.LookPath(t.CLIBin)
			e["cliAvailable"] = err == nil
			e["cliBin"] = t.CLIBin
		}
		list = append(list, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": list})
}

func proxyDeployDispatchHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/proxy-pools/deploy/"), "/")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "butuh /api/proxy-pools/deploy/<target>", "available": proxyDeployTargetNames()})
		return
	}
	t, ok := getProxyDeployTarget(name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "target deploy tak terdaftar: " + name, "available": proxyDeployTargetNames()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body proxyDeployBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	res := t.Build(body)
	if res == nil {
		res = map[string]any{}
	}
	if t.CLIBin != "" {
		_, err := exec.LookPath(t.CLIBin)
		res["cliAvailable"] = err == nil
	}
	poolName := body.Name
	if poolName == "" {
		poolName = body.Project
	}
	if poolName == "" {
		poolName = "flow-router-proxy"
	}
	if d, derr := store.Open(); derr == nil {
		pool := &store.ProxyPool{Name: poolName, Rotation: "single"}
		if uerr := store.UpsertProxyPool(d, pool); uerr == nil {
			res["poolId"] = pool.ID
		}
	}
	writeJSON(w, http.StatusOK, res)
}
