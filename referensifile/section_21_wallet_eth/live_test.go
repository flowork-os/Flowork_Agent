//go:build live

// Live integration test against real Etherscan + CoinGecko APIs.
// Tagged "live" so it doesn't run in normal `go test` — only when explicitly
// requested: `go test -tags live ./internal/wallet/...`
//
// Requires ETHERSCAN_API_KEY env var (set in .env).
package wallet

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLive_Snapshot(t *testing.T) {
	if os.Getenv("ETHERSCAN_API_KEY") == "" {
		t.Skip("ETHERSCAN_API_KEY not set — skipping live test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	p, err := Snapshot(ctx, "0xd129da1c7296a00573b938b2e696bb933a6b7eb1")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	t.Logf("Wallet: %s", p.Address)
	t.Logf("Total USD: $%.2f", p.TotalUSD)
	t.Logf("Holdings: %d", len(p.Holdings))
	if p.PartialErr != "" {
		t.Logf("Partial errors: %s", p.PartialErr)
	}
	for _, h := range p.Holdings {
		t.Logf("  %s %s: %.6f × $%.4f = $%.2f",
			h.ChainName, h.Symbol, h.Amount, h.USDPrice, h.USDValue)
	}
}
