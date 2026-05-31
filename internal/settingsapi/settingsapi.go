// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Settings handlers (owner-level). Audit pass — API key GET masked
//   (4 char), envKeyRe UPPER_SNAKE filter (password hash gak ke-expose/setenv),
//   AI-wallets READ-ONLY host-level (store.Close, no write — isolasi warga
//   utuh), MaxBytesReader, reuse wallet.Snapshot. E2E verified.
//
// Package settingsapi — HTTP handler untuk halaman Settings (owner-level).
//
// Semua operasi di sini menyentuh flowork.db GLOBAL (floworkdb), KECUALI
// AIWalletsHandler yang read-only dari state.db tiap warga (host-level read,
// bukan cross-warga — yang baca proses host/owner, bukan warga lain).
//
// Endpoint:
//
//	GET/POST/DELETE /api/settings/wallet/addresses   — wallet personal owner
//	GET            /api/settings/wallet/portfolio     — portfolio (reuse wallet engine)
//	GET/POST       /api/settings/keys                 — API key global (masked on GET)
//	GET            /api/settings/ai-wallets           — daftar wallet tiap warga (read-only)
package settingsapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/httpx"
	"flowork-gui/internal/wallet"
)

// AgentIDsFunc + OpenAgentStoreFunc — di-wire dari main.go (host). Dipakai
// AIWalletsHandler untuk baca wallet warga read-only.
var (
	AgentIDsFunc       func() []string
	OpenAgentStoreFunc func(agentID string) (*agentdb.Store, error)
	// TestNotifyFunc — di-wire dari main.go (notifyOwnerTelegram). Buat tombol
	// "Test" di Settings → Notifikasi.
	TestNotifyFunc func(ctx context.Context, text string) error
)

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

// API — handler set, di-back floworkdb global store.
type API struct {
	store *floworkdb.Store
}

// New bikin API.
func New(store *floworkdb.Store) *API {
	return &API{store: store}
}

// WalletAddressesHandler — GET/POST/DELETE /api/settings/wallet/addresses
// (wallet PERSONAL owner, disimpan di flowork.db global).
func (a *API) WalletAddressesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.store.ListWalletAddresses()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
	case http.MethodPost:
		var body struct {
			ChainID int    `json:"chain_id"`
			Address string `json:"address"`
			Label   string `json:"label"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		if body.ChainID == 0 || strings.TrimSpace(body.Address) == "" {
			httpx.WriteJSON(w, map[string]any{"error": "chain_id + address required"})
			return
		}
		if err := a.store.AddWalletAddress(body.ChainID, body.Address, body.Label); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	case http.MethodDelete:
		chainID, _ := strconv.Atoi(r.URL.Query().Get("chain_id"))
		address := strings.TrimSpace(r.URL.Query().Get("address"))
		if chainID == 0 || address == "" {
			httpx.WriteJSON(w, map[string]any{"error": "chain_id + address required"})
			return
		}
		if err := a.store.DeleteWalletAddress(chainID, address); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// WalletPortfolioHandler — GET /api/settings/wallet/portfolio?address=
// Reuse wallet.Snapshot. Address kosong → addr pertama owner.
func (a *API) WalletPortfolioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	if address == "" {
		rows, err := a.store.ListWalletAddresses()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if len(rows) == 0 {
			httpx.WriteJSON(w, map[string]any{"error": "no wallet address — tambah address dulu"})
			return
		}
		address = rows[0].Address
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	portfolio, perr := wallet.Snapshot(ctx, address)
	if perr != nil {
		httpx.WriteJSON(w, map[string]any{"error": perr.Error()})
		return
	}
	httpx.WriteJSON(w, portfolio)
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
		if err := a.store.SetSecret(key, body.Value); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		// Live-inject ke env supaya engine (wallet, dll) langsung pakai tanpa restart.
		_ = os.Setenv(key, body.Value)
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// AIWalletsHandler — GET /api/settings/ai-wallets
// Read-only: daftar wallet address tiap warga (host-level read dari state.db
// masing-masing). Bukan cross-warga — proses host yang baca buat display.
func (a *API) AIWalletsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	if AgentIDsFunc == nil || OpenAgentStoreFunc == nil {
		httpx.WriteJSON(w, map[string]any{"error": "agent host not wired"})
		return
	}
	agents := []map[string]any{}
	for _, id := range AgentIDsFunc() {
		entry := map[string]any{"agent_id": id, "addresses": []agentdb.WalletAddress{}}
		store, err := OpenAgentStoreFunc(id)
		if err != nil {
			entry["error"] = err.Error()
			agents = append(agents, entry)
			continue
		}
		rows, lerr := store.ListWalletAddresses()
		store.Close()
		if lerr != nil {
			entry["error"] = lerr.Error()
		} else {
			entry["addresses"] = rows
		}
		agents = append(agents, entry)
	}
	httpx.WriteJSON(w, map[string]any{"items": agents, "count": len(agents)})
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
