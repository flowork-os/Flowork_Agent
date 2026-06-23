// 🔒 FROZEN tools-core — Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// JANGAN edit tanpa unfreeze SADAR owner (di-hash KERNEL_FREEZE.md + chattr +i). Baca lock/tools.md DULU.
// Mau NAMBAH kapabilitas? bikin file feature_*.go BARU (init→RegisterFeature) — JANGAN bongkar file ini.
//
// feature_tools_sidecar.go — FASE-B: SIDECAR TOOLS (plug-and-play ala WordPress).
//
// Discover folder `tools/<name>/` (tiap tool = modul+binary native SENDIRI, lib di folder sendiri) →
// register tiap binary sbg tool DINAMIS (di-exec host sbg proses terpisah, isolasi). Nambah fitur =
// file feature_*.go BARU (init→RegisterFeature) — main.go frozen GA disentuh.
//
// Lihat: internal/toolsidecar/toolsidecar.go · tools/README.md · docs/ROADMAP_MULTI_OS_TOOLS.md §SIDECAR.
package main

import (
	"log"
	"net/http"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/toolsidecar"
)

func init() {
	RegisterFeature(Feature{Name: "tools-sidecar", Phase: PhaseSeed, Apply: func(d *Deps) {
		dir := toolsidecar.ToolsDir()
		n, names, unbuilt := toolsidecar.DiscoverAndRegister(dir)
		log.Printf("tools-sidecar: %d tool ke-register dari %s %v (belum-build: %v)", n, dir, names, unbuilt)

		// GET = list status · POST = RELOAD (re-discover abis `tools/build-tools.sh`, tanpa restart host).
		d.Mux.HandleFunc("/api/tools/sidecar", func(w http.ResponseWriter, r *http.Request) {
			rn, _, runbuilt := toolsidecar.DiscoverAndRegister(dir)
			httpx.WriteJSON(w, map[string]any{
				"dir": dir, "registered": rn, "unbuilt": runbuilt,
				"tools": toolsidecar.Specs(), // {name,capability,description,params} buat GUI
				"note":  "tool sidecar = folder self-contained di tools/<name>/ (lib sendiri, binary terpisah, exec-isolasi, akses semua agent)",
			})
		})
	}})
}
