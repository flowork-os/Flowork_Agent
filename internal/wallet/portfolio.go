// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 21 phase 1 — Portfolio aggregator (Etherscan + CoinGecko
//   union per chain → Holding[] + total USD). Phase 2 (multi-address sum,
//   txn history, tax loss harvest) → tambah file baru.

package wallet

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"
)

// Holding is one line item in the wallet portfolio: a single token balance
// on a single chain, with USD conversion.
type Holding struct {
	ChainID   int     `json:"chain_id"`
	ChainName string  `json:"chain_name"`
	Symbol    string  `json:"symbol"`
	Amount    float64 `json:"amount"`             // human-readable (already divided by decimals)
	USDPrice  float64 `json:"usd_price"`          // current USD price per unit
	USDValue  float64 `json:"usd_value"`          // amount × usd_price
	Contract  string  `json:"contract,omitempty"` // "" for native coin
	IsNative  bool    `json:"is_native"`
}

// Portfolio is the complete wallet snapshot — all holdings across all
// chains plus the total USD value. Matches Trust Wallet's home view.
type Portfolio struct {
	Address    string    `json:"address"`
	FetchedAt  time.Time `json:"fetched_at"`
	Holdings   []Holding `json:"holdings"`
	TotalUSD   float64   `json:"total_usd"`
	RecentTx   []Tx      `json:"recent_tx,omitempty"`
	PartialErr string    `json:"partial_error,omitempty"` // non-fatal errors from per-chain calls
}

// Snapshot fetches the full portfolio for address across all supported
// chains: native balance + monitored ERC20 balances + USD prices.
//
// Errors are best-effort: if Polygon fails but Ethereum succeeds, we
// return what we have plus a PartialErr message. Only total failure
// (no holdings at all) returns an error.
func Snapshot(ctx context.Context, address string) (*Portfolio, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		address = strings.TrimSpace(os.Getenv("TRUST_WALLET_ADDRESS"))
	}
	if address == "" {
		return nil, fmt.Errorf("wallet address not provided and TRUST_WALLET_ADDRESS env unset")
	}

	es, err := NewEtherscan()
	if err != nil {
		return nil, err
	}
	cg := NewCoinGecko()

	// Step 1: fetch all prices upfront (1 CoinGecko call, shared).
	prices, priceErr := cg.Prices(ctx, AllCGIDs())
	if prices == nil {
		prices = make(map[string]float64)
	}

	p := &Portfolio{
		Address:   address,
		FetchedAt: time.Now().UTC(),
	}
	var errs []string
	if priceErr != nil {
		errs = append(errs, "coingecko: "+priceErr.Error())
	}

	// Step 2: for each supported chain, fetch native + each ERC20.
	for _, chain := range Supported() {
		// Native balance
		wei, err := es.Balance(ctx, chain.ID, address)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s native: %v", chain.Name, err))
		} else if wei != nil {
			amount := weiToFloat(wei, 18)
			price := prices[chain.NativeCGID]
			p.Holdings = append(p.Holdings, Holding{
				ChainID:   chain.ID,
				ChainName: chain.Name,
				Symbol:    chain.NativeSymbol,
				Amount:    amount,
				USDPrice:  price,
				USDValue:  amount * price,
				IsNative:  true,
			})
		}

		// ERC20 tokens on this chain — serialize with 220ms delay to stay
		// under Etherscan free-tier 5 req/s. Parallelisation hits rate limit.
		for _, tok := range TokensFor(chain.ID) {
			raw, err := es.TokenBalance(ctx, chain.ID, tok.Contract, address)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s %s: %v", chain.Name, tok.Symbol, err))
				continue
			}
			if raw == nil || raw.Sign() == 0 {
				// Skip zero balances from the list — no reason to clutter.
				continue
			}
			amount := weiToFloat(raw, tok.Decimals)
			price := prices[tok.CGID]
			p.Holdings = append(p.Holdings, Holding{
				ChainID:   chain.ID,
				ChainName: chain.Name,
				Symbol:    tok.Symbol,
				Amount:    amount,
				USDPrice:  price,
				USDValue:  amount * price,
				Contract:  tok.Contract,
				IsNative:  false,
			})
			// Throttle: 220ms between token calls (≤5 req/s free tier).
			select {
			case <-ctx.Done():
				errs = append(errs, "context canceled mid-fetch")
				break
			case <-time.After(220 * time.Millisecond):
			}
		}
	}

	// Compute total.
	for _, h := range p.Holdings {
		p.TotalUSD += h.USDValue
	}

	// Sort holdings by USD value descending (biggest first, like Trust Wallet).
	sort.Slice(p.Holdings, func(i, j int) bool {
		return p.Holdings[i].USDValue > p.Holdings[j].USDValue
	})

	if len(errs) > 0 {
		p.PartialErr = strings.Join(errs, "; ")
	}

	if len(p.Holdings) == 0 && p.PartialErr != "" {
		return p, fmt.Errorf("wallet snapshot failed entirely: %s", p.PartialErr)
	}

	return p, nil
}

// RecentTxAll collects recent native + ERC20 tx across all chains, sorted
// newest-first. Useful for the "Recent transactions" section of the panel.
// Limit applies PER chain per type (so final output can be up to
// len(Supported()) × 2 × limit entries).
func RecentTxAll(ctx context.Context, address string, limit int) ([]Tx, error) {
	if limit <= 0 {
		limit = 10
	}
	address = strings.TrimSpace(address)
	if address == "" {
		address = strings.TrimSpace(os.Getenv("TRUST_WALLET_ADDRESS"))
	}
	if address == "" {
		return nil, fmt.Errorf("wallet address not provided")
	}
	es, err := NewEtherscan()
	if err != nil {
		return nil, err
	}
	var all []Tx
	for _, chain := range Supported() {
		// Native tx
		nt, err := es.RecentTx(ctx, chain.ID, address, limit)
		if err == nil {
			all = append(all, nt...)
		}
		// ERC20 tx
		tt, err := es.RecentTokenTx(ctx, chain.ID, address, limit)
		if err == nil {
			all = append(all, tt...)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})
	// Cap to avoid dumping every chain's full history.
	cap := limit * 3
	if len(all) > cap {
		all = all[:cap]
	}
	return all, nil
}

// weiToFloat converts a raw big.Int balance to float64 given decimals.
// Float64 has 15-17 decimal digits of precision — enough for display but
// callers should NOT use it for signing / amount math. For that, keep
// big.Int.
func weiToFloat(n *big.Int, decimals int) float64 {
	if n == nil {
		return 0
	}
	// Convert via big.Float for precision.
	bf := new(big.Float).SetInt(n)
	divisor := new(big.Float).SetInt(powTen(decimals))
	bf.Quo(bf, divisor)
	f, _ := bf.Float64()
	return f
}

// powTen returns 10^n as *big.Int.
func powTen(n int) *big.Int {
	r := big.NewInt(1)
	ten := big.NewInt(10)
	for i := 0; i < n; i++ {
		r.Mul(r, ten)
	}
	return r
}
