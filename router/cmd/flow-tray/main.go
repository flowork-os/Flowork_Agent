// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

//go:build !cgo_tray
// +build !cgo_tray

package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintln(os.Stderr, "flow-tray (CGO-free placeholder build)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Detected OS: %s\n", runtime.GOOS)
	switch runtime.GOOS {
	case "windows":
		fmt.Fprintln(os.Stderr, "For Windows, prefer the PowerShell tray:")
		fmt.Fprintln(os.Stderr, "  powershell -NoProfile -ExecutionPolicy Bypass -File scripts/tray-win.ps1")
	case "linux", "darwin":
		fmt.Fprintln(os.Stderr, "For a real menu-bar icon on this OS, build with CGO + systray:")
		fmt.Fprintln(os.Stderr, "  go get github.com/getlantern/systray")
		fmt.Fprintln(os.Stderr, "  go build -tags cgo_tray -o flow-tray ./cmd/flow-tray")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Otherwise use the CGO-free launcher:")
		switch runtime.GOOS {
		case "linux":
			fmt.Fprintln(os.Stderr, "  scripts/tray-linux.sh {status|open|restart}")
		case "darwin":
			fmt.Fprintln(os.Stderr, "  scripts/tray-mac.sh {status|open|restart}")
		}
	default:
		fmt.Fprintln(os.Stderr, "Native tray not yet scaffolded for this OS.")
	}
	os.Exit(0)
}
