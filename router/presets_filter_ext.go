// presets_filter_ext.go — SIBLING ext (⚠️ FROZEN 2026-07-02 seizin owner —
// behavior stabil dikunci). 📄 Dok: lock/connect-prune.md
// Rapihin daftar preset "Connect Provider" (owner: "sisain Antigravity + Claude +
// API, CLI lain ga bisa dites"). Sembunyiin preset CLI-login yg owner ga punya buat
// dites; tambah preset Antigravity (AuthType subscription — login OAuth Imports).
// Switch FLOWORK_PRESET_PRUNE=0 → tampilin semua (ubah behavior TANPA buka gembok).
package main

import (
	"os"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/store"
)

// hiddenPresetIDs — preset CLI-login/subscription yg BELUM bisa dites owner
// (ga punya akunnya). Bukan dihapus dari kode (frozen) — cuma disembunyiin di UI.
var hiddenPresetIDs = map[string]bool{
	"kiro-ai": true, "opencode": true, "codeium-plus": true,
	"windsurf-cascade": true, "jetbrains-ai": true, "zed-ai": true,
}

func presetPruneEnabled() bool {
	v := strings.TrimSpace(os.Getenv("FLOWORK_PRESET_PRUNE"))
	return v == "" || v == "1" || strings.EqualFold(v, "true") // default ON
}

func init() {
	PresetsHook = func(in []store.Preset) []store.Preset {
		if !presetPruneEnabled() {
			return in
		}
		out := make([]store.Preset, 0, len(in)+1)
		// Antigravity di ATAS (proven, login via OAuth Imports).
		out = append(out, store.Preset{
			ID:          "antigravity",
			Name:        "Antigravity (Google Gemini)",
			Icon:        "🚀",
			Description: "Gemini 3.1 Pro via Google cloud-code (Antigravity). Login: tab OAuth Imports → 'Login with Google'. Butuh app Antigravity ke-install.",
			Provider:    "antigravity",
			// subscription: login OAuth (tab OAuth Imports), NO API key diketik —
			// biar muncul di filter "Subscription" (sekelas Claude Pro/Max) & form
			// connect ga salah minta key. Token asli di-auto-capture backend.
			AuthType:    store.AuthTypeSubscription,
			Format:      "antigravity",
			BaseURL:     "https://cloudcode-pa.googleapis.com",
			Priority:    5,
			Models:      []string{"gemini-3.1-pro-low", "gemini-3.5-flash-low"},
			Tag:         "proven",
		})
		for _, p := range in {
			if hiddenPresetIDs[p.ID] {
				continue
			}
			out = append(out, p)
		}
		return out
	}
}
