// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package cryptocheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

var goplusChain = map[string]string{
	"eth": "1", "ethereum": "1", "bsc": "56", "bnb": "56", "binance": "56",
	"polygon": "137", "matic": "137", "arbitrum": "42161", "arb": "42161",
	"base": "8453", "optimism": "10", "op": "10", "avalanche": "43114", "avax": "43114",
	"fantom": "250", "ftm": "250", "cronos": "25", "zksync": "324", "linea": "59144",
}

type Flag struct {
	Severity string `json:"severity"`
	Label    string `json:"label"`
	Detail   string `json:"detail,omitempty"`
}

type Report struct {
	Chain     string         `json:"chain"`
	Address   string         `json:"address"`
	Source    string         `json:"source"`
	RiskLevel string         `json:"risk_level"`
	Score     int            `json:"score"`
	Flags     []Flag         `json:"flags"`
	Summary   string         `json:"summary"`
	Token     map[string]any `json:"token,omitempty"`
}

func CheckToken(chain, address string) (*Report, error) {
	chain = strings.ToLower(strings.TrimSpace(chain))
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("address token wajib")
	}
	if chain == "solana" || chain == "sol" {
		return checkSolana(address)
	}
	if id, ok := goplusChain[chain]; ok {
		return checkEVM(chain, id, address)
	}
	return nil, fmt.Errorf("chain %q belum didukung (EVM: eth/bsc/polygon/base/arbitrum/… atau solana)", chain)
}

func getJSON(url string) (map[string]any, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Flowork-CheckToken/1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))[:min(160, len(body))])
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return m, nil
}

func checkEVM(chain, chainID, address string) (*Report, error) {
	url := fmt.Sprintf("https://api.gopluslabs.io/api/v1/token_security/%s?contract_addresses=%s",
		chainID, strings.ToLower(address))
	m, err := getJSON(url)
	if err != nil {
		return nil, fmt.Errorf("GoPlus: %w", err)
	}
	result, _ := m["result"].(map[string]any)
	var r map[string]any
	for _, v := range result {
		if mm, ok := v.(map[string]any); ok {
			r = mm
			break
		}
	}
	rep := &Report{Chain: chain, Address: address, Source: "GoPlus", RiskLevel: "UNKNOWN"}
	if r == nil {
		rep.Summary = "GoPlus tidak punya data untuk token ini (kontrak baru / chain salah / belum ter-index). Hati-hati: data nol ≠ aman."
		rep.Flags = append(rep.Flags, Flag{"high", "no-data", "GoPlus belum punya data keamanan token ini"})
		rep.Score = 50
		return rep, nil
	}
	s := func(k string) string { v, _ := r[k].(string); return strings.TrimSpace(v) }
	is := func(k string) bool { return s(k) == "1" }
	taxF := func(k string) float64 { f, _ := strconv.ParseFloat(s(k), 64); return f }

	rep.Token = map[string]any{
		"name": s("token_name"), "symbol": s("token_symbol"),
		"holder_count": s("holder_count"), "total_supply": s("total_supply"),
	}

	if is("is_honeypot") {
		rep.Flags = append(rep.Flags, Flag{"critical", "honeypot", "token bisa dibeli tapi TIDAK bisa dijual"})
	}
	if is("cannot_sell_all") {
		rep.Flags = append(rep.Flags, Flag{"critical", "cannot-sell-all", "holder tidak bisa jual semua token"})
	}
	if is("selfdestruct") {
		rep.Flags = append(rep.Flags, Flag{"critical", "selfdestruct", "kontrak bisa self-destruct"})
	}
	if is("transfer_pausable") {
		rep.Flags = append(rep.Flags, Flag{"high", "transfer-pausable", "transfer bisa di-pause owner (bisa kunci dana)"})
	}

	if is("is_mintable") {
		rep.Flags = append(rep.Flags, Flag{"high", "mintable", "supply bisa di-mint owner (dilusi/rug)"})
	}
	if is("is_proxy") {
		rep.Flags = append(rep.Flags, Flag{"high", "proxy", "kontrak proxy upgradeable (logic bisa diganti)"})
	}
	if is("hidden_owner") {
		rep.Flags = append(rep.Flags, Flag{"high", "hidden-owner", "ada owner tersembunyi"})
	}
	if is("can_take_back_ownership") {
		rep.Flags = append(rep.Flags, Flag{"high", "reclaim-ownership", "ownership bisa di-ambil-balik"})
	}
	if s("is_open_source") == "0" {
		rep.Flags = append(rep.Flags, Flag{"high", "not-verified", "source code TIDAK terverifikasi"})
	}
	if bt := taxF("buy_tax"); bt > 0.10 {
		rep.Flags = append(rep.Flags, Flag{"high", "high-buy-tax", fmt.Sprintf("buy tax %.0f%%", bt*100)})
	}
	if st := taxF("sell_tax"); st > 0.10 {
		rep.Flags = append(rep.Flags, Flag{"high", "high-sell-tax", fmt.Sprintf("sell tax %.0f%%", st*100)})
	}

	if is("owner_change_balance") {
		rep.Flags = append(rep.Flags, Flag{"medium", "owner-change-balance", "owner bisa ubah saldo holder"})
	}
	if is("is_blacklisted") {
		rep.Flags = append(rep.Flags, Flag{"medium", "blacklist-fn", "ada fungsi blacklist (alamat bisa diblokir jual)"})
	}
	if is("trading_cooldown") {
		rep.Flags = append(rep.Flags, Flag{"medium", "trading-cooldown", "ada cooldown trading"})
	}
	if is("is_anti_whale") && is("anti_whale_modifiable") {
		rep.Flags = append(rep.Flags, Flag{"medium", "anti-whale-modifiable", "limit anti-whale bisa diubah owner"})
	}
	if hc, _ := strconv.Atoi(s("holder_count")); hc > 0 && hc < 50 {
		rep.Flags = append(rep.Flags, Flag{"medium", "few-holders", fmt.Sprintf("holder sangat sedikit (%d)", hc)})
	}

	rep.score()
	return rep, nil
}

func checkSolana(mint string) (*Report, error) {
	url := "https://api.rugcheck.xyz/v1/tokens/" + mint + "/report/summary"
	m, err := getJSON(url)
	if err != nil {
		return nil, fmt.Errorf("RugCheck: %w", err)
	}
	rep := &Report{Chain: "solana", Address: mint, Source: "RugCheck", RiskLevel: "UNKNOWN"}
	if risks, ok := m["risks"].([]any); ok {
		for _, ri := range risks {
			rm, _ := ri.(map[string]any)
			if rm == nil {
				continue
			}
			name, _ := rm["name"].(string)
			desc, _ := rm["description"].(string)
			level, _ := rm["level"].(string)
			sev := "medium"
			switch strings.ToLower(level) {
			case "danger":
				sev = "critical"
			case "warn", "warning":
				sev = "high"
			}
			rep.Flags = append(rep.Flags, Flag{sev, name, desc})
		}
	}
	if sc, ok := m["score"].(float64); ok {
		rep.Token = map[string]any{"rugcheck_score": sc}
	}
	rep.score()
	return rep, nil
}

func (r *Report) score() {
	crit, high, med := 0, 0, 0
	for _, f := range r.Flags {
		switch f.Severity {
		case "critical":
			crit++
		case "high":
			high++
		case "medium":
			med++
		}
	}
	r.Score = min(100, crit*50+high*20+med*8)
	switch {
	case crit > 0:
		r.RiskLevel = "SCAM"
	case high >= 2:
		r.RiskLevel = "HIGH-RISK"
	case high == 1 || med >= 2:
		r.RiskLevel = "CAUTION"
	case r.RiskLevel == "UNKNOWN":
		r.RiskLevel = "LOOKS-OK"
	}

	rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "info": 3}
	sort.SliceStable(r.Flags, func(i, j int) bool { return rank[r.Flags[i].Severity] < rank[r.Flags[j].Severity] })

	switch r.RiskLevel {
	case "SCAM":
		r.Summary = fmt.Sprintf("⛔ SCAM/BERBAHAYA — %d temuan kritis. JANGAN beli. (%s)", crit, r.Source)
	case "HIGH-RISK":
		r.Summary = fmt.Sprintf("🔴 RISIKO TINGGI — %d flag berat. Sangat hati-hati. (%s)", high, r.Source)
	case "CAUTION":
		r.Summary = fmt.Sprintf("🟠 WASPADA — ada flag. DYOR dulu. (%s)", r.Source)
	case "LOOKS-OK":
		r.Summary = fmt.Sprintf("🟢 Tidak ada red-flag mayor di %s — TAPI ini BUKAN jaminan aman, tetap DYOR.", r.Source)
	default:
		r.Summary = "Data tidak cukup untuk menilai."
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
