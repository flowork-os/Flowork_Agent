// guardian_handler.go — HTTP untuk Guardian FASE 1 (owner-session gated via authMgr.Middleware).
// status (baca) · arm (rekam baseline) · disarm (butuh password ulang — anti session-hijack/XSS).
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/floworkauth"
	"flowork-gui/internal/guardian"
	"flowork-gui/internal/kernel/loader"
)

// GET /api/guardian/status — status guardian buat GUI panel.
func guardianStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		v, _ := guardian.Load()
		tfWriteJSON(w, 0, map[string]any{
			"armed":       v.Armed,
			"mode":        v.Mode,
			"safe_mode":   guardian.SafeMode(),
			"sealed_at":   v.SealedAt,
			"protected":   len(v.Baseline),
			"sealed":      v.Sealed,
			"seal_method": v.SealMethod,
		})
	}
}

// POST /api/guardian/arm — rekam baseline (hash binary + file inti) + armed=true.
func guardianArmHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		v, err := guardian.Arm(guardian.CoreFilesFromManifest(), now)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "armed": true, "protected": len(v.Baseline), "sealed_at": now, "sealed": v.Sealed, "seal_method": v.SealMethod})
	}
}

// POST /api/guardian/disarm {password} — matikan guardian. WAJIB password ulang (defense-in-depth:
// session bisa di-hijack/XSS; disarm = aksi keamanan tinggi). Verifikasi reuse floworkauth (beku).
func guardianDisarmHandler(authMgr *floworkauth.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var b struct {
			Password string `json:"password"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&b)
		// Verifikasi password lewat Login (reuse auth beku); sukses → buang sesi sekali-pakai.
		tok, _, lerr := authMgr.Login(b.Password)
		if lerr != nil {
			tfWriteJSON(w, http.StatusUnauthorized, map[string]any{"error": "password salah"})
			return
		}
		authMgr.Logout(tok)
		if err := guardian.Disarm(); err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, map[string]any{"ok": true, "armed": false})
	}
}

// guardianBootCheck — dipanggil saat boot dari main. Kalau armed & integritas gagal → SAFE-MODE
// + alert owner. Reuse notifyOwnerTelegram. Lintas-OS, no-root.
func guardianBootCheck() {
	v, err := guardian.Load()
	if err != nil || !v.Armed {
		return // pasif (dev mode / belum di-arm)
	}
	ok, problems := v.Verify()
	if ok {
		return
	}
	guardian.EnterSafeMode()
	log.Printf("guardian: TAMPER terdeteksi — SAFE-MODE aktif. Artefak bermasalah: %v", problems)
	msg := "🛡️ GUARDIAN: integritas kernel GAGAL — SAFE-MODE aktif (exec/install diblok).\n\nBerubah/hilang:\n• " +
		joinUpTo(problems, 12)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = notifyOwnerTelegram(ctx, msg)
	}()
}

// guardianDangerCaps — CapSource buat sentinel (FASE 3): enumerate tiap agent → daftar cap
// BERBAHAYA yang dideklarasi. Baca manifest.json (capabilities_required) + loket.json (consumes)
// di tiap <agentsdir>/*. Sentinel bandingin snapshot ini → cap berbahaya BARU = eskalasi → alert.
func guardianDangerCaps() map[string][]string {
	out := map[string][]string{}
	root := loader.AgentsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".fwagent")
		dir := filepath.Join(root, e.Name())
		var caps []string
		if raw, rerr := os.ReadFile(filepath.Join(dir, "manifest.json")); rerr == nil {
			var m struct {
				CapabilitiesRequired []string `json:"capabilities_required"`
			}
			if json.Unmarshal(raw, &m) == nil {
				for _, c := range m.CapabilitiesRequired {
					if pluginCapDangerous(c) {
						caps = append(caps, c)
					}
				}
			}
		}
		if raw, rerr := os.ReadFile(filepath.Join(dir, "loket.json")); rerr == nil {
			var m struct {
				Consumes []string `json:"consumes"`
			}
			if json.Unmarshal(raw, &m) == nil {
				for _, c := range m.Consumes {
					if loketCapDangerous(c) {
						caps = append(caps, c)
					}
				}
			}
		}
		if len(caps) > 0 {
			out[id] = caps
		}
	}
	return out
}

// loketCapDangerous — cap loket yang ngasih kuasa nyata (GrantOwner-class).
func loketCapDangerous(c string) bool {
	switch c {
	case "exec.run", "http.fetch", "fs.read", "fs.write", "fs.list", "tool.run", "slash.run":
		return true
	}
	return false
}

func joinUpTo(items []string, max int) string {
	out := ""
	for i, s := range items {
		if i >= max {
			out += "\n• …(+" + itoa(len(items)-max) + " lagi)"
			break
		}
		if i > 0 {
			out += "\n• "
		}
		out += s
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
