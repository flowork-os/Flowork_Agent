package wallet

import (
	"math/big"
	"testing"
)

func TestWeiToFloat(t *testing.T) {
	cases := []struct {
		wei      *big.Int
		decimals int
		want     float64
	}{
		{big.NewInt(0), 18, 0},
		{big.NewInt(1_000_000_000_000_000_000), 18, 1.0}, // 1 ETH
		{big.NewInt(500_000_000_000_000_000), 18, 0.5},   // 0.5 ETH
		{big.NewInt(1_000_000), 6, 1.0},                  // 1 USDT (6 decimals)
		{big.NewInt(12_345_678), 6, 12.345678},           // USDT fractional
	}
	for _, c := range cases {
		got := weiToFloat(c.wei, c.decimals)
		if diff := got - c.want; diff > 0.00001 || diff < -0.00001 {
			t.Errorf("weiToFloat(%s, %d) = %f, want %f", c.wei, c.decimals, got, c.want)
		}
	}
}

func TestSupportedChains(t *testing.T) {
	chains := Supported()
	if len(chains) < 3 {
		t.Errorf("expected at least 3 supported chains, got %d", len(chains))
	}
	names := map[string]bool{}
	for _, c := range chains {
		names[c.Name] = true
		if c.ID == 0 || c.NativeCGID == "" || c.NativeSymbol == "" {
			t.Errorf("chain %+v has missing fields", c)
		}
	}
	for _, required := range []string{"Ethereum", "Polygon", "Arbitrum"} {
		if !names[required] {
			t.Errorf("missing required chain: %s", required)
		}
	}
}

func TestMonitoredTokens_AllChainsHaveStablecoin(t *testing.T) {
	tokens := MonitoredTokens()
	byChain := map[int][]Token{}
	for _, tk := range tokens {
		byChain[tk.ChainID] = append(byChain[tk.ChainID], tk)
	}
	// Every supported chain should have at least USDT + USDC
	for _, chain := range Supported() {
		list := byChain[chain.ID]
		if len(list) < 2 {
			t.Errorf("chain %s has only %d tokens, want >= 2", chain.Name, len(list))
		}
		hasUSDT, hasUSDC := false, false
		for _, tk := range list {
			if tk.Symbol == "USDT" {
				hasUSDT = true
			}
			if tk.Symbol == "USDC" {
				hasUSDC = true
			}
		}
		if !hasUSDT {
			t.Errorf("chain %s missing USDT", chain.Name)
		}
		if !hasUSDC {
			t.Errorf("chain %s missing USDC", chain.Name)
		}
	}
}

func TestAllCGIDs_Dedup(t *testing.T) {
	ids := AllCGIDs()
	seen := map[string]int{}
	for _, id := range ids {
		seen[id]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("duplicate CGID in AllCGIDs: %s appears %d times", id, count)
		}
	}
	// Must contain at least ethereum + tether + usd-coin
	required := []string{"ethereum", "tether", "usd-coin"}
	for _, r := range required {
		if seen[r] == 0 {
			t.Errorf("AllCGIDs missing required id: %s", r)
		}
	}
}

func TestPowTen(t *testing.T) {
	if powTen(0).Int64() != 1 {
		t.Errorf("powTen(0) != 1")
	}
	if powTen(6).Int64() != 1_000_000 {
		t.Errorf("powTen(6) != 1M")
	}
	if powTen(18).String() != "1000000000000000000" {
		t.Errorf("powTen(18) wrong")
	}
}

func TestTokensFor(t *testing.T) {
	eth := TokensFor(1)
	if len(eth) < 2 {
		t.Errorf("Ethereum tokens < 2: got %d", len(eth))
	}
	// Unknown chain → empty
	none := TokensFor(9999)
	if len(none) != 0 {
		t.Errorf("unknown chain should return empty, got %d", len(none))
	}
}
