package apps

import (
	"os/exec"
	"strings"
	"testing"
)

// TestFlowAlphaApp — proves the FlowAlpha quant app runs end-to-end via the HOST (no auth, no
// LLM): manifest parse → python core spawn → line-JSON protocol → InvokeOp routing → live
// data → indicators → backtest → shared state. Ops are registered as agent tools (the "two
// drivers" side). Network-tolerant: if the data source is blocked/offline, it skips.
func TestFlowAlphaApp(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	m := NewManager("../../apps")
	if err := m.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	defer m.Shutdown()

	if _, ok := m.Get("flowalpha"); !ok {
		t.Fatal("app 'flowalpha' not loaded")
	}
	if len(m.regs["flowalpha"]) == 0 {
		t.Fatal("flowalpha operations not registered as agent tools")
	}

	netSkip := func(err error) bool {
		return strings.Contains(err.Error(), "data source") || strings.Contains(err.Error(), "urlopen")
	}

	// run_backtest is the headline closed-loop Op: data → SMA → strategy → metrics + equity.
	res, err := m.InvokeOp("flowalpha", "run_backtest",
		map[string]any{"symbol": "BTCUSDT", "interval": "1h", "limit": 300, "fast": 10, "slow": 30}, "agent")
	if err != nil {
		if netSkip(err) {
			t.Skipf("data source unreachable (skip): %v", err)
		}
		t.Fatalf("run_backtest: %v", err)
	}
	bt, _ := res.(map[string]any)
	metrics, _ := bt["metrics"].(map[string]any)
	if metrics == nil || metrics["total_return_pct"] == nil {
		t.Fatalf("backtest metrics missing: %+v", res)
	}
	eq, _ := bt["equity"].([]any)
	if len(eq) < 50 {
		t.Fatalf("equity curve too short: %d", len(eq))
	}
	t.Logf("backtest BTCUSDT sma_cross → return=%v%% sharpe=%v trades=%v",
		metrics["total_return_pct"], metrics["sharpe"], metrics["trades"])

	// Shared state: get_last_backtest (driver = human-gui) sees the run the agent just made.
	last, err := m.InvokeOp("flowalpha", "get_last_backtest", nil, "human-gui")
	if err != nil {
		t.Fatalf("get_last_backtest: %v", err)
	}
	if lm, _ := last.(map[string]any); lm["metrics"] == nil {
		t.Fatalf("shared state did not return the last backtest: %+v", last)
	}

	// An unregistered op MUST be rejected (gate validation).
	if _, e := m.InvokeOp("flowalpha", "rm_rf", nil, "agent"); e == nil {
		t.Fatal("unregistered op must be rejected")
	}
}
