// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "flow_router"

func DefaultDataDir() string {
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			home, _ := os.UserHomeDir()
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appdata, appName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "."+appName)
}

func DataDir() string {
	for _, k := range []string{"FLOW_ROUTER_DATA", "DATA_DIR"} {
		v := os.Getenv(k)
		if v == "" {
			continue
		}
		err := os.MkdirAll(v, 0o700)
		if err == nil {
			return v
		}
		if pe, ok := err.(*os.PathError); ok && pe.Err == os.ErrPermission {
			log.Printf("[DATA_DIR] %q not writable, falling back to default", v)
		}
	}
	return DefaultDataDir()
}

func MITMDir() string { return filepath.Join(DataDir(), "mitm") }
