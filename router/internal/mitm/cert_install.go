// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func InstallRootCA() (manualHint string, err error) {
	certPath := filepath.Join(MITMDir(), "rootCA.pem")
	if _, statErr := os.Stat(certPath); statErr != nil {
		return "", fmt.Errorf("rootCA.pem not found at %s — run the router once to generate it", certPath)
	}
	switch runtime.GOOS {
	case "darwin":
		return installMacOS(certPath)
	case "linux":
		return installLinux(certPath)
	case "windows":
		return installWindows(certPath)
	}
	return "", fmt.Errorf("cert install not supported on %s", runtime.GOOS)
}

func UninstallRootCA() (manualHint string, err error) {
	switch runtime.GOOS {
	case "darwin":
		return uninstallMacOS()
	case "linux":
		return uninstallLinux()
	case "windows":
		return uninstallWindows()
	}
	return "", fmt.Errorf("cert uninstall not supported on %s", runtime.GOOS)
}

func installMacOS(certPath string) (string, error) {
	manual := "sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain " + certPath
	c := exec.Command("sudo", "-n", "security", "add-trusted-cert", "-d", "-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain", certPath)
	if out, err := c.CombinedOutput(); err != nil {
		return manual, fmt.Errorf("install failed (need sudo): %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "", nil
}

func uninstallMacOS() (string, error) {
	manual := "sudo security delete-certificate -c 'flow_router Root CA' /Library/Keychains/System.keychain"
	c := exec.Command("sudo", "-n", "security", "delete-certificate", "-c", "flow_router Root CA",
		"/Library/Keychains/System.keychain")
	if out, err := c.CombinedOutput(); err != nil {
		return manual, fmt.Errorf("uninstall failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "", nil
}

func installLinux(certPath string) (string, error) {

	dest := "/usr/local/share/ca-certificates/flow_router-root.crt"
	manual := fmt.Sprintf("sudo cp %s %s && sudo update-ca-certificates", certPath, dest)
	if !IsSudoAvailable() {
		return manual, fmt.Errorf("sudo is required to copy %s into the system trust store", dest)
	}
	if out, err := exec.Command("sudo", "-n", "cp", certPath, dest).CombinedOutput(); err != nil {
		return manual, fmt.Errorf("copy failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("sudo", "-n", "update-ca-certificates").CombinedOutput(); err != nil {
		return manual, fmt.Errorf("update-ca-certificates failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "", nil
}

func uninstallLinux() (string, error) {
	dest := "/usr/local/share/ca-certificates/flow_router-root.crt"
	manual := fmt.Sprintf("sudo rm -f %s && sudo update-ca-certificates --fresh", dest)
	if out, err := exec.Command("sudo", "-n", "rm", "-f", dest).CombinedOutput(); err != nil {
		return manual, fmt.Errorf("remove failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("sudo", "-n", "update-ca-certificates", "--fresh").CombinedOutput(); err != nil {
		return manual, fmt.Errorf("update-ca-certificates --fresh failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "", nil
}

func installWindows(certPath string) (string, error) {
	manual := fmt.Sprintf(`certutil -addstore -f Root "%s"`, certPath)
	if !IsAdmin() {

		if err := RunElevatedPowerShell(`Start-Process certutil -ArgumentList @('-addstore','-f','Root','` + certPath + `') -Verb RunAs -Wait`); err == nil {
			return "", nil
		}
		return manual, fmt.Errorf("admin rights required; run from an elevated shell: %s", manual)
	}
	if out, err := exec.Command("certutil", "-addstore", "-f", "Root", certPath).CombinedOutput(); err != nil {
		return manual, fmt.Errorf("certutil failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "", nil
}

func uninstallWindows() (string, error) {
	manual := `certutil -delstore Root "flow_router Root CA"`
	if !IsAdmin() {
		if err := RunElevatedPowerShell(`Start-Process certutil -ArgumentList @('-delstore','Root','flow_router Root CA') -Verb RunAs -Wait`); err == nil {
			return "", nil
		}
		return manual, fmt.Errorf("admin rights required; run: %s", manual)
	}
	if out, err := exec.Command("certutil", "-delstore", "Root", "flow_router Root CA").CombinedOutput(); err != nil {
		return manual, fmt.Errorf("certutil failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return "", nil
}
