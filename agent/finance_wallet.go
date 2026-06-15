// === LOCKED FILE (soft) === Status: STABLE — DO NOT MODIFY without owner approval (LOCKED ≠ FREEZE).
// Owner: Aola Sahidin (Mr.Dev) · Locked 2026-06-16. Reason: R8 fase-1 wallet settings. VERIFIED E2E:
// GET default; invalid-addr ditolak; set→privkey MASKED (field ga dibalikin); DELETE wipe+disable.
// Private key di SECRET store (lowercase→ga env-inject), address publik di KV. Transaksi nyata=fase-2.
//
// finance_wallet.go — R8 SELF-FINANCE fase-1: menu setting kredensial wallet (owner-approved
// 2026-06-16). Visi: organisme biayai diri sendiri (bayar token LLM + cari duit) → butuh wallet
// SENDIRI. Fase-1 = MENU + STORAGE AMAN aja (transaksi nyata = fase-2 pas wallet siap dipakai).
//
// Wallet EVM (0x..., Ethereum-compatible). KEAMANAN: address = PUBLIK (boleh keliatan); PRIVATE
// KEY = secret store floworkdb (masked GET, GA PERNAH dibalikin, GA ke-env-inject krn nama lowercase,
// GA ke-commit). Hard-limit = pagar pengaman (cap belanja). Owner-gated (loopback + auth). File
// terisolasi (plug-and-play): finance rusak → sentuh sini doang, ga ganggu settings core (locked).

package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"flowork-gui/internal/floworkdb"
)

// secret (private key) — nama LOWERCASE biar GA ke-auto-inject ke env saat boot (main.go:450
// cuma inject UPPER_SNAKE). Private key wallet cukup dibaca eksplisit pas transaksi (fase-2).
const financePrivKeyKey = "finance_wallet_privkey"

// KV (non-secret config).
const (
	financeEnabledKey  = "finance_enabled"
	financeChainKey    = "finance_chain"
	financeAddressKey  = "finance_wallet_address"
	financeRPCKey      = "finance_rpc"
	financeCurrencyKey = "finance_currency"
	financeLimitKey    = "finance_hard_limit"
)

// evmAddrRe — alamat EVM valid: 0x + 40 hex.
var evmAddrRe = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)

func financeMask(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}

// financeWalletHandler — GET (masked) / POST (set) / DELETE (wipe key + disable) kredensial wallet.
//
//	GET    → {enabled, chain, address, rpc, currency, hard_limit, privkey_masked, has_privkey, balance}
//	POST   → {enabled?, chain?, address?, rpc?, currency?, hard_limit?, privkey?} — privkey kosong =
//	         JANGAN overwrite (biar masked GET ga nimpa secret asli). address divalidasi EVM.
//	DELETE → cabut private key + enabled=0 (organisme ga bisa transaksi).
func financeWalletHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, err := floworkdb.Shared()
		if err != nil {
			tfWriteJSON(w, 0, map[string]any{"error": "db: " + err.Error()})
			return
		}
		switch r.Method {
		case http.MethodGet:
			pk, _ := db.GetSecret(financePrivKeyKey)
			enabled, _ := db.GetKV(financeEnabledKey)
			addr, _ := db.GetKV(financeAddressKey)
			chain, _ := db.GetKV(financeChainKey)
			rpc, _ := db.GetKV(financeRPCKey)
			cur, _ := db.GetKV(financeCurrencyKey)
			lim, _ := db.GetKV(financeLimitKey)
			tfWriteJSON(w, 0, map[string]any{
				"enabled":        enabled == "1",
				"chain":          chain,
				"address":        addr, // publik — aman ditampilin
				"rpc":            rpc,
				"currency":       cur,
				"hard_limit":     lim,
				"privkey_masked": financeMask(pk),
				"has_privkey":    strings.TrimSpace(pk) != "",
				"balance":        "(belum tersambung — transaksi nyata fase-2 pas wallet siap)",
				"note":           "Private key disimpan lokal terenkripsi (~/.flowork), GA pernah ditampilkan/di-commit. Hard-limit = pagar belanja maksimum. Owner pegang penuh.",
			})
		case http.MethodPost:
			var b struct {
				Enabled   *bool   `json:"enabled"`
				Chain     *string `json:"chain"`
				Address   *string `json:"address"`
				RPC       *string `json:"rpc"`
				Currency  *string `json:"currency"`
				HardLimit *string `json:"hard_limit"`
				PrivKey   *string `json:"privkey"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&b); err != nil {
				tfWriteJSON(w, 0, map[string]any{"error": "invalid json: " + err.Error()})
				return
			}
			if b.Address != nil {
				addr := strings.TrimSpace(*b.Address)
				if addr != "" && !evmAddrRe.MatchString(addr) {
					tfWriteJSON(w, 0, map[string]any{"error": "address bukan EVM valid (0x + 40 hex)"})
					return
				}
				_ = db.SetKV(financeAddressKey, addr)
			}
			if b.Chain != nil {
				_ = db.SetKV(financeChainKey, strings.TrimSpace(*b.Chain))
			}
			if b.RPC != nil {
				_ = db.SetKV(financeRPCKey, strings.TrimSpace(*b.RPC))
			}
			if b.Currency != nil {
				_ = db.SetKV(financeCurrencyKey, strings.TrimSpace(*b.Currency))
			}
			if b.HardLimit != nil {
				// validasi angka (cap belanja). kosong = 0 (default aman = ga boleh belanja).
				hl := strings.TrimSpace(*b.HardLimit)
				if hl != "" {
					if _, perr := strconv.ParseFloat(hl, 64); perr != nil {
						tfWriteJSON(w, 0, map[string]any{"error": "hard_limit harus angka"})
						return
					}
				}
				_ = db.SetKV(financeLimitKey, hl)
			}
			if b.Enabled != nil {
				v := "0"
				if *b.Enabled {
					v = "1"
				}
				_ = db.SetKV(financeEnabledKey, v)
			}
			// Private key cuma di-update kalau diisi (kosong = pertahankan; anti silent-wipe lewat masked GET).
			if b.PrivKey != nil && strings.TrimSpace(*b.PrivKey) != "" {
				if err := db.SetSecret(financePrivKeyKey, strings.TrimSpace(*b.PrivKey)); err != nil {
					tfWriteJSON(w, 0, map[string]any{"error": err.Error()})
					return
				}
			}
			tfWriteJSON(w, 0, map[string]any{"ok": true})
		case http.MethodDelete:
			_ = db.DeleteSecret(financePrivKeyKey)
			_ = db.SetKV(financeEnabledKey, "0")
			tfWriteJSON(w, 0, map[string]any{"ok": true, "note": "private key dicabut + wallet di-disable"})
		default:
			tfWriteJSON(w, 0, map[string]any{"error": "method not allowed"})
		}
	}
}
