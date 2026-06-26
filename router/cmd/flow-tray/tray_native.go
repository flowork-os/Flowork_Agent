// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

//go:build cgo_tray
// +build cgo_tray

package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	url := os.Getenv("FLOW_ROUTER_URL")
	if url == "" {
		url = "http://127.0.0.1:2402"
	}
	fmt.Fprintln(os.Stderr, "flow-tray native build — dependency stub")
	fmt.Fprintln(os.Stderr, "Run: go get github.com/getlantern/systray && rebuild")
	fmt.Fprintln(os.Stderr, "Opening dashboard:", url)
	_ = exec.Command("xdg-open", url).Start()
	_ = exec.Command("open", url).Start()
}
