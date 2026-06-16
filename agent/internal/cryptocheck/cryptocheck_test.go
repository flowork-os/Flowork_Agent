package cryptocheck

import "testing"

// Live tests (hit real public APIs). Skip with -short.
func TestCheckToken_USDC_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skip live network test")
	}
	rep, err := CheckToken("eth", "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48") // USDC (legit)
	if err != nil {
		t.Skipf("network/API unavailable: %v", err)
	}
	t.Logf("USDC eth → risk=%s score=%d flags=%d | %s", rep.RiskLevel, rep.Score, len(rep.Flags), rep.Summary)
	if rep.RiskLevel == "SCAM" {
		t.Errorf("USDC false-positive SCAM: %+v", rep.Flags)
	}
	if rep.Source != "GoPlus" {
		t.Errorf("expected GoPlus source, got %s", rep.Source)
	}
}

func TestCheckToken_BadInput(t *testing.T) {
	if _, err := CheckToken("eth", ""); err == nil {
		t.Error("empty address should error")
	}
	if _, err := CheckToken("dogechain-unknown", "0x123"); err == nil {
		t.Error("unsupported chain should error")
	}
}
