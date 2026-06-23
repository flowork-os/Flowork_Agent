//go:build (linux || darwin || windows) && !android

// browser_desktop_ext.go — CABANG (extension point) NON-FROZEN buat browser_desktop.go
// yang FROZEN.
//
// ⚖️ ATURAN ABADI (owner Mr.Dev, 2026-06-23): file yang udah di-FREEZE TIDAK BOLEH dibuka
// lagi buat nambah filtur. SEMUA tuning/filtur browser masuk SINI. browser_desktop.go
// (frozen) cuma MANGGIL fungsi di file ini → ga pernah disentuh lagi.
//
// 📖 WAJIB BACA: /home/mrflow/Documents/FLowork_os/lock/browser.md sebelum ngutak-atik
// browser-control (cara kerja, daftar tool, cookie-inject, env, cara nambah tool).
//
// === CARA NAMBAH FILTUR BROWSER (tanpa buka file frozen) ===
//  1. TOOL BROWSER BARU (mis. browser_scroll): bikin FILE BARU `browser_<nama>.go`
//     (build-tag sama) dgn `func init(){ tools.Register(&browserXxxTool{}) }`. Go
//     ngegabung semua init() sepaket → tool baru ke-daftar TANPA edit browser_desktop.go.
//  2. TUNING LAUNCH/LIFECYCLE: pakai env switch di bawah (headless, flags, idle timeout).
package builtins

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
)

// browserIdleTimeout — berapa lama browser nganggur sebelum di-close otomatis (anti
// zombie chromium numpuk). Default 30 menit. Override TANPA unfreeze:
// env FLOWORK_BROWSER_IDLE_MIN (menit, mis. "15"). <=0 / invalid → default.
func browserIdleTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_BROWSER_IDLE_MIN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 30 * time.Minute
}

// browserHeadless — true (default) = chromium headless (server/OS-image). Buat DEBUG
// pengen lihat jendela: env FLOWORK_BROWSER_HEADLESS=0 (headful). TANPA unfreeze.
func browserHeadless() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_BROWSER_HEADLESS"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

// applyExtraBrowserFlags — nambah flag chromium tambahan (boolean) ke launcher TANPA
// buka file frozen. env FLOWORK_BROWSER_FLAGS = daftar flag dipisah koma (mis.
// "disable-extensions,mute-audio"). Kosong = ga nambah apa-apa (perilaku default).
func applyExtraBrowserFlags(l *launcher.Launcher) *launcher.Launcher {
	raw := strings.TrimSpace(os.Getenv("FLOWORK_BROWSER_FLAGS"))
	if raw == "" {
		return l
	}
	for _, f := range strings.Split(raw, ",") {
		if f = strings.TrimSpace(f); f != "" {
			l = l.Set(flags.Flag(f))
		}
	}
	return l
}
