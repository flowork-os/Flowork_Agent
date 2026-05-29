package tui

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// runFloworkMesh mencari binary flowork-mesh di sebelah executable aktif
// atau di PATH, lalu eksekusi dengan subcommand tertentu (status/start/join).
// Timeout 15 detik biar /mesh status tidak nge-block TUI kalau daemon
// tidak nyala. Stdout+stderr digabung.
func runFloworkMesh(workspace string, args ...string) (string, error) {
	bin := "flowork-mesh"
	if runtime.GOOS == "windows" {
		bin = "flowork-mesh.exe"
	}
	// rc147 fix per binary policy Ayah: PRIMARY = <workspace>/build/<bin>,
	// bukan PATH lookup. Sebelumnya `exec.LookPath("flowork")` ambigu — bisa
	// resolve ke binary di root project (yang udah dilarang ada per rc141)
	// atau directory PATH lain yang stale. Prefer eksplisit build/.
	cand := filepath.Join(workspace, "build", bin)
	if _, statErr := os.Stat(cand); statErr == nil {
		bin = cand
	} else if exe, err := exec.LookPath("flowork"); err == nil {
		// Fallback: directory binary flowork TUI (kalau dijalankan dari
		// folder yang beda dari workspace, atau install global).
		cand2 := filepath.Join(filepath.Dir(exe), bin)
		if _, err := os.Stat(cand2); err == nil {
			bin = cand2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// openURL membuka URL di browser default OS. Tidak menunggu proses exit —
// dipanggil dari slash command, balik instan.
//
// gemini-bug#2 fix (2026-04-19): Windows path tidak lagi lewat `cmd /c start`
// yang rentan terhadap command injection via URL berisi `&` atau `|`.
// Sekarang pakai `rundll32 url.dll,FileProtocolHandler` yang treat input
// sebagai URL literal — satu argv, tanpa shell interpretation.
func openURL(url string) error {
	// Validasi scheme supaya tidak accidental exec selain http/https/file.
	if !isSafeOpenURL(url) {
		return fmt.Errorf("openURL: scheme tidak didukung (hanya http/https/file)")
	}
	switch runtime.GOOS {
	case "windows":
		// rundll32 pass argv langsung — TIDAK lewat cmd.exe shell.
		// `url.dll,FileProtocolHandler` standard Windows API, aman untuk URL.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// isSafeOpenURL reports apakah URL aman dibuka dengan handler OS default.
// Bug #11 fix: removed file:// scheme — on Windows, FileProtocolHandler
// executes .exe/.bat files pointed to by file:// URIs, enabling local
// command execution if an attacker injects a malicious file:// link.
func isSafeOpenURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	low := strings.ToLower(raw)
	return strings.HasPrefix(low, "http://") ||
		strings.HasPrefix(low, "https://")
}

// openChromeAppWindow membuka Chrome/Edge dalam mode --app=url (tanpa
// tab/address bar) — bikin web chat terasa seperti desktop app. Windows
// default ke PATH chrome atau msedge; fallback ke openURL biasa.
func openChromeAppWindow(url string) error {
	flag := "--app=" + url
	candidates := []string{"chrome", "google-chrome", "msedge", "brave"}
	if runtime.GOOS == "windows" {
		// Path standar install Chrome/Edge di Windows.
		candidates = append([]string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		}, candidates...)
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return exec.Command(c, flag).Start()
		}
		// Cek absolute path langsung juga.
		cmd := exec.Command(c, flag)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("chrome/edge/brave tidak ditemukan di PATH")
}

// detectLANIP menebak IP LAN non-loopback lewat UDP trick — tanpa packet
// real dikirim, OS otomatis memilih interface keluar untuk destination
// public. Return "" kalau gagal (misal tidak ada network).
func detectLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}
