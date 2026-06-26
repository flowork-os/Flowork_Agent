// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func IsAdmin() bool {
	if runtime.GOOS != "windows" {
		return os.Geteuid() == 0
	}
	out, err := exec.Command("whoami", "/groups").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "S-1-16-12288")
}

func RunElevatedPowerShell(command string) error {
	if runtime.GOOS != "windows" {
		return os.ErrInvalid
	}

	starter := `Start-Process powershell -Verb RunAs -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-Command','` +
		strings.ReplaceAll(command, "'", "''") + `'`
	return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", starter).Run()
}
