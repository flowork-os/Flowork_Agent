// guardian_handler.go — HTTP untuk Guardian FASE 1 (owner-session gated via authMgr.Middleware).
// status (baca) · arm (rekam baseline) · disarm (butuh password ulang — anti session-hijack/XSS).
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"flowork-gui/internal/floworkauth"
	"flowork-gui/internal/guardian"
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
