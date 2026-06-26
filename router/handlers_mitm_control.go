// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Tab GUI: MITM Proxy → dok lock/gui/MITM Proxy.md  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/mitm"
)

var (
	mitmMgrMu sync.Mutex
	mitmMgr   *mitm.Manager
)

func mitmListenAddr(reqAddr string) string {
	if reqAddr != "" {
		return reqAddr
	}
	if v := os.Getenv("FLOW_ROUTER_MITM_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:443"
}

func mitmStartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Addr      string `json:"addr"`
		HijackDNS bool   `json:"hijackDNS"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	mitmMgrMu.Lock()
	defer mitmMgrMu.Unlock()
	if mitmMgr != nil && mitm.IsRunning() {
		writeJSON(w, http.StatusOK, map[string]any{"started": true, "already": true, "pid": mitm.ReadPidFile()})
		return
	}
	cm, err := mitm.NewCertManager(mitm.DataDir())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"started": false, "error": "cert manager: " + err.Error()})
		return
	}

	var hosts []string
	if body.HijackDNS {
		hosts = mitm.TargetHosts
	}
	mgr := mitm.NewManager(mitmListenAddr(body.Addr), cm, hosts)
	if err := mgr.Start(nil); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"started": false, "error": err.Error(), "addr": mitmListenAddr(body.Addr)})
		return
	}
	mitmMgr = mgr
	writeJSON(w, http.StatusOK, map[string]any{"started": true, "addr": mitmListenAddr(body.Addr), "pid": mitm.ReadPidFile(), "dnsHijacked": body.HijackDNS})
}

func mitmStopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mitmMgrMu.Lock()
	defer mitmMgrMu.Unlock()
	if mitmMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"stopped": true, "already": true})
		return
	}
	err := mitmMgr.Stop()
	mitmMgr = nil
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"stopped": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": true})
}

func stopMITMOnShutdown() {
	mitmMgrMu.Lock()
	defer mitmMgrMu.Unlock()
	if mitmMgr != nil {
		_ = mitmMgr.Stop()
		mitmMgr = nil
	}
}
