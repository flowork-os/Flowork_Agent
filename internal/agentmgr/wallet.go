// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 21 phase 1 wallet endpoints. Portfolio fetch via wallet
//   package — read-only, no signing, no private key. ETHERSCAN_API_KEY
//   required (env). Phase 2 (snapshot cron, multi-address aggregation,
//   non-Etherscan chain) → tambah file baru.
//
// wallet.go — Section 21 phase 1: portfolio + addresses endpoints.

package agentmgr

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/httpx"
	"flowork-gui/internal/wallet"
)

// WalletAddressesHandler — GET/POST/DELETE /api/agents/wallet/addresses?id=<agent>
//   GET    → list rows
//   POST   → body {chain_id, address, label}
//   DELETE → query ?chain_id=&address=
func WalletAddressesHandler(w http.ResponseWriter, r *http.Request) {
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	switch r.Method {
	case http.MethodGet:
		rows, err := store.ListWalletAddresses()
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
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
			return
		}
		if body.ChainID == 0 || body.Address == "" {
			httpx.WriteJSON(w, map[string]any{"error": "chain_id + address required"})
			return
		}
		if err := store.AddWalletAddress(body.ChainID, body.Address, body.Label); err != nil {
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
		if err := store.DeleteWalletAddress(chainID, address); err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		httpx.WriteJSON(w, map[string]any{"ok": true})
	default:
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
	}
}

// WalletPortfolioHandler — GET /api/agents/wallet/portfolio?id=<agent>&address=<addr>
//
// Address optional — kalau kosong, pakai first stored address. ETHERSCAN_API_KEY
// env required. Save snapshot setelah fetch sukses.
func WalletPortfolioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	if address == "" {
		rows, err := store.ListWalletAddresses()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if len(rows) == 0 {
			httpx.WriteJSON(w, map[string]any{"error": "no wallet address — POST /addresses first or pass ?address="})
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
	// Append snapshot best-effort.
	portfolioJSON, _ := json.Marshal(portfolio)
	if _, ierr := store.InsertWalletSnapshot(portfolio.TotalUSD, string(portfolioJSON)); ierr != nil {
		portfolio.PartialErr += "; snapshot insert: " + ierr.Error()
	}
	httpx.WriteJSON(w, portfolio)
}

// WalletSnapshotsHandler — GET /api/agents/wallet/snapshots?id=<agent>&limit=
func WalletSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("id"))
	if agentID == "" {
		httpx.WriteJSON(w, map[string]any{"error": "agent id required"})
		return
	}
	limit := 30
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	store, err := openAgentStore(agentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListWalletSnapshots(limit)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}
