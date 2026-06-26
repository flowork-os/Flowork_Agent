// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

var TestNotifyFunc func(ctx context.Context, text string) error

const (
	notifyTokenKey = "NOTIFY_TG_TOKEN"
	notifyChatKey  = "notify_tg_chat"
)

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

var envKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

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

type API struct {
	store *floworkdb.Store
}

func New(store *floworkdb.Store) *API {
	return &API{store: store}
}

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
			if !envKeyRe.MatchString(k) {
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

		if strings.TrimSpace(body.Value) == "" {
			httpx.WriteJSON(w, map[string]any{"error": "value kosong — isi nilai baru, atau pakai tombol Hapus buat ngebuang key"})
			return
		}
		if err := a.store.SetSecret(key, body.Value); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}

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

var modelRe = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,64}$`)

func (a *API) RouterDefaultHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		model, _ := a.store.GetKV("llm_default_model")
		url, _ := a.store.GetKV("router_default_url")
		httpx.WriteJSON(w, map[string]any{"model": model, "router_url": url})
	case http.MethodPost:
		var body struct {
			Model     string `json:"model"`
			RouterURL string `json:"router_url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		model := strings.TrimSpace(body.Model)
		url := strings.TrimSpace(body.RouterURL)
		if model != "" && !modelRe.MatchString(model) {
			httpx.WriteJSON(w, map[string]any{"error": "model id invalid (letters/digits/_-.: only, e.g. claude-haiku-4-5)"})
			return
		}
		if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			httpx.WriteJSON(w, map[string]any{"error": "router_url must start with http:// or https://"})
			return
		}
		if err := a.store.SetKV("llm_default_model", model); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if err := a.store.SetKV("router_default_url", url); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}

		if url != "" {
			_ = os.Setenv("ROUTER_DEFAULT_URL", url)
		} else {
			_ = os.Unsetenv("ROUTER_DEFAULT_URL")
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

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
