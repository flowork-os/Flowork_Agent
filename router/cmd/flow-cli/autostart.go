// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const autostartName = "flow_router"

func cmdAutostart(args []string) error {
	op := "status"
	if len(args) > 0 {
		op = args[0]
	}
	switch op {
	case "enable":
		return autostartEnable()
	case "disable":
		return autostartDisable()
	case "status":
		return autostartStatus()
	default:
		return fmt.Errorf("usage: flow-cli autostart {enable|disable|status}")
	}
}

func autostartEnable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "linux":
		return linuxAutostartWrite(exe)
	case "darwin":
		return macAutostartWrite(exe)
	case "windows":
		return winAutostartWrite(exe)
	}
	return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
}

func autostartDisable() error {
	switch runtime.GOOS {
	case "linux":
		return os.Remove(linuxAutostartPath())
	case "darwin":
		return os.Remove(macAutostartPath())
	case "windows":
		return winAutostartRemove()
	}
	return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
}

func autostartStatus() error {
	switch runtime.GOOS {
	case "linux":
		return reportStat(linuxAutostartPath())
	case "darwin":
		return reportStat(macAutostartPath())
	case "windows":
		return winAutostartStatus()
	}
	return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
}

func linuxAutostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", autostartName+".desktop")
}

func linuxAutostartWrite(exe string) error {
	p := linuxAutostartPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	body := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=flow_router",
		"Comment=Local AI router",
		"Exec=" + exe,
		"X-GNOME-Autostart-enabled=true",
		"NoDisplay=false",
		"",
	}, "\n")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		return err
	}
	fmt.Println("autostart enabled:", p)
	return nil
}

func macAutostartPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.flow_router.plist")
}

func macAutostartWrite(exe string) error {
	p := macAutostartPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.flow_router</string>
    <key>ProgramArguments</key><array><string>` + exe + `</string></array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><false/>
</dict>
</plist>
`
	if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
		return err
	}

	_ = exec.Command("launchctl", "load", p).Run()
	fmt.Println("autostart enabled:", p)
	return nil
}

func winAutostartWrite(exe string) error {
	ps := fmt.Sprintf(
		`Set-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name '%s' -Value '"%s"'`,
		autostartName, strings.ReplaceAll(exe, `'`, `''`))
	c := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	fmt.Println("autostart enabled (HKCU Run entry)")
	return nil
}

func winAutostartRemove() error {
	ps := fmt.Sprintf(
		`Remove-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name '%s' -ErrorAction SilentlyContinue`,
		autostartName)
	c := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	fmt.Println("autostart removed")
	return nil
}

func winAutostartStatus() error {
	ps := fmt.Sprintf(
		`(Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name '%s' -ErrorAction SilentlyContinue).'%s'`,
		autostartName, autostartName)
	out, err := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps).Output()
	if err != nil {
		return err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		fmt.Println("autostart: not configured")
		return nil
	}
	fmt.Println("autostart:", s)
	return nil
}

func reportStat(path string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Println("autostart: ON →", path)
		return nil
	}
	fmt.Println("autostart: off")
	return nil
}
