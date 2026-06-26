// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/flowork-os/flowork_Router/internal/mitm"
)

func mitmStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dataDir := mitm.DataDir()
	mitmDir := mitm.MITMDir()
	certPath := filepath.Join(mitmDir, "rootCA.pem")
	out := map[string]any{
		"dataDir":     dataDir,
		"mitmDir":     mitmDir,
		"certPath":    certPath,
		"isRunning":   mitm.IsRunning(),
		"pid":         mitm.ReadPidFile(),
		"isAdmin":     mitm.IsAdmin(),
		"targetHosts": mitm.TargetHosts,
		"toolMap":     mitmToolHostMap(),
	}
	if st, err := os.Stat(certPath); err == nil {
		out["certBytes"] = st.Size()
		out["certExists"] = true
	} else {
		out["certExists"] = false
	}
	if hijack, err := mitm.CheckDNSStatus(mitm.TargetHosts); err == nil {
		out["dnsHijacked"] = hijack
	}
	out["hostsPath"] = mitm.HostsFilePath()
	writeJSON(w, http.StatusOK, out)
}

func mitmToolHostMap() map[string]string {
	out := map[string]string{}
	for _, h := range mitm.TargetHosts {
		if tool := mitm.GetToolForHost(h); tool != "" {
			out[h] = tool
		}
	}
	return out
}

func mitmRootCADownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	certPath := filepath.Join(mitm.MITMDir(), "rootCA.pem")
	pem, err := os.ReadFile(certPath)
	if err != nil {

		if _, gerr := mitm.NewCertManager(mitm.DataDir()); gerr != nil {
			http.Error(w, "rootCA not available: "+gerr.Error(), http.StatusServiceUnavailable)
			return
		}
		pem, err = os.ReadFile(certPath)
		if err != nil {
			http.Error(w, "rootCA still missing: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="flow_router-rootCA.pem"`)
	_, _ = w.Write(pem)
}

func mitmInstallCAHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hint, err := mitm.InstallRootCA()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"installed": false, "error": err.Error(), "manualCommand": hint})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installed": true})
}

func mitmUninstallCAHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hint, err := mitm.UninstallRootCA()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"removed": false, "error": err.Error(), "manualCommand": hint})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

func mitmDNSAddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Hosts []string `json:"hosts"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if len(body.Hosts) == 0 {
		body.Hosts = mitm.TargetHosts
	}
	if err := mitm.AddDNSEntries(body.Hosts); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"added": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": true, "hosts": body.Hosts})
}

func mitmDNSRemoveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := mitm.RemoveAllDNSEntries(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"removed": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}
