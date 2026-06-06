// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31 (re-audited 2026-06-07)
// Reason: Settings handlers (owner-level). Audit pass — API key GET masked
//
//	(4 char), envKeyRe UPPER_SNAKE filter (password hash not exposed/setenv),
//	IsSensitiveEnvKey blocklist (refuse PATH/LD_*/DYLD_*/FLOWORK_*/… injection),
//	MaxBytesReader. E2E verified. (Wallet feature removed 2026-06-06.)
//
// Update 2026-06-07 (owner-approved audit): KeysHandler POST now refuses an empty
//	value — the GUI clears the value field on Edit, so a stray Save would otherwise
//	overwrite the real secret with "" (silent wipe). Removal is an explicit DELETE.
//	Tested (keys_empty_test.go).
//
// Package settingsapi — HTTP handler untuk halaman Settings (owner-level).
// Semua operasi menyentuh flowork.db GLOBAL (floworkdb), tidak per-warga.
//
// Endpoint:
//
//	GET/POST/DELETE /api/settings/keys      — API key global (masked on GET, reserved-env refused)
//	GET/POST        /api/settings/notify    — Telegram owner notification (token masked)
//	GET/POST        /api/settings/youtube*  — YouTube OAuth status + credentials + config
package settingsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/httpx"
)

// TestNotifyFunc — di-wire dari main.go (notifyOwnerTelegram). Buat tombol
// "Test" di Settings → Notifikasi.
var TestNotifyFunc func(ctx context.Context, text string) error

// keys floworkdb untuk notif Telegram owner-level (TERPISAH dari secret agent
// — agent tetap isolated/plug-and-play).
const (
	notifyTokenKey = "NOTIFY_TG_TOKEN" // secrets
	notifyChatKey  = "notify_tg_chat"  // kv
)

// NotifyHandler — GET/POST /api/settings/notify
//
//	GET  → {bot_token_masked, chat_id, set}
//	POST → {bot_token?, chat_id, test?} simpan ke flowork.db; bot_token kosong
//	       = jangan overwrite (biar masked GET ngga nimpa). test=true → kirim
//	       pesan tes ke owner.
func (a *API) NotifyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		token, _ := a.store.GetSecret(notifyTokenKey)
		chat, _ := a.store.GetKV(notifyChatKey)
		httpx.WriteJSON(w, map[string]any{
			"bot_token_masked": mask(token),
			"chat_id":          chat,
			"set":              strings.TrimSpace(token) != "" && strings.TrimSpace(chat) != "",
		})
	case http.MethodPost:
		var body struct {
			BotToken string `json:"bot_token"`
			ChatID   string `json:"chat_id"`
			Test     bool   `json:"test"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		// Token cuma di-update kalau diisi (kosong = pertahankan yang lama).
		if strings.TrimSpace(body.BotToken) != "" {
			if err := a.store.SetSecret(notifyTokenKey, strings.TrimSpace(body.BotToken)); err != nil {
				httpx.WriteJSON(w, map[string]any{"error": err.Error()})
				return
			}
		}
		if err := a.store.SetKV(notifyChatKey, strings.TrimSpace(body.ChatID)); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if body.Test {
			if TestNotifyFunc == nil {
				httpx.WriteJSON(w, map[string]any{"ok": true, "test": "notifier not wired"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			if err := TestNotifyFunc(ctx, "✅ Test notif Flowork — Telegram owner terhubung. Scanner bakal kirim alert ke sini."); err != nil {
				httpx.WriteJSON(w, map[string]any{"ok": false, "error": "test gagal: " + err.Error()})
				return
			}
			httpx.WriteJSON(w, map[string]any{"ok": true, "test": "sent"})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// envKeyRe — API key valid = nama env var (UPPER_SNAKE). Menyaring juga biar
// owner_password_hash (lowercase) gak ikut ke-expose / ke-setenv.
var envKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// IsSensitiveEnvKey reports whether an env-var name must NEVER be settable as an
// owner "API key" — because it would change how the process or its child commands
// run (loader/PATH hijack, or forging the kernel's own loopback identity). A key
// like ETHERSCAN_API_KEY is fine; PATH / LD_PRELOAD / FLOWORK_LOOPBACK_SECRET are not.
// Used by BOTH the POST handler and the boot loader (defense at write AND re-inject).
func IsSensitiveEnvKey(k string) bool {
	switch k {
	case "PATH", "HOME", "SHELL", "IFS", "ENV", "BASH_ENV", "GOROOT", "GOPATH",
		"HOSTALIASES", "TERMINFO", "PYTHONPATH", "PERL5LIB", "NODE_OPTIONS", "TMPDIR":
		return true
	}
	for _, p := range []string{"LD_", "DYLD_", "FLOWORK_", "NSS_", "GIT_"} {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// API — handler set, di-back floworkdb global store.
type API struct {
	store *floworkdb.Store
}

// New bikin API.
func New(store *floworkdb.Store) *API {
	return &API{store: store}
}

// KeysHandler — GET/POST /api/settings/keys
//
//	GET  → daftar key + nilai MASKED (4 char terakhir).
//	POST → {key,value} simpan ke secrets + os.Setenv langsung (live).
func (a *API) KeysHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keys, err := a.store.ListSecretKeys()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		items := []map[string]any{}
		for _, k := range keys {
			if !envKeyRe.MatchString(k) { // skip owner_password_hash dll
				continue
			}
			v, _ := a.store.GetSecret(k)
			items = append(items, map[string]any{
				"key":    k,
				"masked": mask(v),
				"set":    strings.TrimSpace(v) != "",
			})
		}
		httpx.WriteJSON(w, map[string]any{"items": items, "count": len(items)})
	case http.MethodPost:
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		key := strings.TrimSpace(body.Key)
		if !envKeyRe.MatchString(key) {
			httpx.WriteJSON(w, map[string]any{"error": "key harus UPPER_SNAKE_CASE (nama env var, mis. ETHERSCAN_API_KEY)"})
			return
		}
		if IsSensitiveEnvKey(key) {
			httpx.WriteJSON(w, map[string]any{"error": "key " + key + " is reserved (loader/PATH/kernel env) — refused"})
			return
		}
		// Refuse an empty value: the GUI clears the value field on "Edit", so a
		// stray Save would otherwise overwrite the real secret with "" (silent
		// wipe). Removing a key is an explicit DELETE, never an empty POST.
		if strings.TrimSpace(body.Value) == "" {
			httpx.WriteJSON(w, map[string]any{"error": "value kosong — isi nilai baru, atau pakai tombol Hapus buat ngebuang key"})
			return
		}
		if err := a.store.SetSecret(key, body.Value); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		// Live-inject ke env supaya engine (wallet, dll) langsung pakai tanpa restart.
		_ = os.Setenv(key, body.Value)
		httpx.WriteJSON(w, map[string]any{"ok": true})
	case http.MethodDelete:
		key := strings.TrimSpace(r.URL.Query().Get("key"))
		if !envKeyRe.MatchString(key) {
			httpx.WriteJSON(w, map[string]any{"error": "key invalid"})
			return
		}
		if err := a.store.DeleteSecret(key); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		_ = os.Unsetenv(key)
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// mask — tampilkan 4 char terakhir, sisanya bullet.
func mask(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return strings.Repeat("•", len(v))
	}
	return strings.Repeat("•", 6) + v[len(v)-4:]
}
