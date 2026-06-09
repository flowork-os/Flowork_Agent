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

	// Multi-asset: a stock symbol (AAPL) routes to Yahoo, not Binance.
	if sp, err := m.InvokeOp("flowalpha", "get_price", map[string]any{"symbol": "AAPL"}, "agent"); err == nil {
		if spm, _ := sp.(map[string]any); spm["price"] == nil || spm["asset"] != "stock/fx" {
			t.Fatalf("stock price wrong: %+v", sp)
		}
		t.Logf("multi-asset: AAPL price=%v (%v)", sp.(map[string]any)["price"], sp.(map[string]any)["asset"])
	} else if !netSkip(err) && !strings.Contains(err.Error(), "urlopen") {
		t.Logf("AAPL price skipped (data source): %v", err)
	}

	// custom_indicator (safe AST formula → series). Security (malicious rejection) is covered
	// by the standalone core test; here we confirm a valid formula yields a series.
	if ci, err := m.InvokeOp("flowalpha", "custom_indicator", map[string]any{"symbol": "BTCUSDT", "formula": "sma(close,10)-sma(close,30)", "limit": 80}, "agent"); err == nil {
		if cm, _ := ci.(map[string]any); cm["series"] == nil {
			t.Fatalf("custom_indicator wrong shape: %+v", ci)
		}
	} else if !netSkip(err) {
		t.Fatalf("custom_indicator: %v", err)
	}

	// list_strategies + run_optimize (parameter sweep → best params).
	if ls, err := m.InvokeOp("flowalpha", "list_strategies", nil, "agent"); err == nil {
		if lm, _ := ls.(map[string]any); lm["strategies"] == nil {
			t.Fatalf("list_strategies wrong shape: %+v", ls)
		}
	}
	opt, err := m.InvokeOp("flowalpha", "run_optimize",
		map[string]any{"symbol": "BTCUSDT", "interval": "1h", "limit": 300, "strategy": "sma_cross"}, "agent")
	if err != nil {
		if netSkip(err) {
			t.Skipf("data source unreachable (skip): %v", err)
		}
		t.Fatalf("run_optimize: %v", err)
	}
	om, _ := opt.(map[string]any)
	best, _ := om["best"].(map[string]any)
	if best == nil || best["metrics"] == nil {
		t.Fatalf("optimize best missing: %+v", opt)
	}
	t.Logf("optimize sma_cross tested=%v best=%v", om["tested"], best["params"])

	// Shared state: get_last_backtest (driver = human-gui) sees the run the agent just made.
	last, err := m.InvokeOp("flowalpha", "get_last_backtest", nil, "human-gui")
	if err != nil {
		t.Fatalf("get_last_backtest: %v", err)
	}
	if lm, _ := last.(map[string]any); lm["metrics"] == nil {
		t.Fatalf("shared state did not return the last backtest: %+v", last)
	}

	// ai_analyze via the Flowork router (sovereign). Skip if the router is offline.
	if ai, err := m.InvokeOp("flowalpha", "ai_analyze", map[string]any{"symbol": "BTCUSDT", "interval": "1h", "limit": 200}, "agent"); err == nil {
		if am, _ := ai.(map[string]any); am["analysis"] == nil {
			t.Fatalf("ai_analyze wrong shape: %+v", ai)
		}
		t.Log("ai_analyze via router: OK")
	} else if strings.Contains(err.Error(), "router unreachable") || netSkip(err) {
		t.Log("ai_analyze skipped (router offline)")
	} else {
		t.Fatalf("ai_analyze: %v", err)
	}

	// Paper portfolio (shared state): reset → buy → portfolio reflects the position.
	if _, err := m.InvokeOp("flowalpha", "paper_reset", nil, "agent"); err != nil {
		t.Fatalf("paper_reset: %v", err)
	}
	if _, err := m.InvokeOp("flowalpha", "paper_buy", map[string]any{"symbol": "BTCUSDT", "quote_amount": 2500.0}, "agent"); err != nil {
		if netSkip(err) {
			t.Skipf("data source unreachable (skip): %v", err)
		}
		t.Fatalf("paper_buy: %v", err)
	}
	pf, err := m.InvokeOp("flowalpha", "portfolio_get", nil, "human-gui")
	if err != nil {
		t.Fatalf("portfolio_get: %v", err)
	}
	pm, _ := pf.(map[string]any)
	poss, _ := pm["positions"].([]any)
	if len(poss) != 1 {
		t.Fatalf("expected 1 paper position after buy, got %+v", pf)
	}
	if cash, _ := pm["cash"].(float64); !(cash > 7000 && cash < 7600) {
		t.Fatalf("cash after $2500 buy off: %v", pm["cash"])
	}
	t.Logf("paper portfolio: equity=%v cash=%v positions=%d", pm["equity"], pm["cash"], len(poss))

	// Watchlist + price alerts (shared state).
	if _, err := m.InvokeOp("flowalpha", "watchlist_add", map[string]any{"symbol": "BTCUSDT"}, "agent"); err != nil {
		t.Fatalf("watchlist_add: %v", err)
	}
	if wl, err := m.InvokeOp("flowalpha", "watchlist_get", nil, "human-gui"); err == nil {
		if wm, _ := wl.(map[string]any); wm["watchlist"] == nil {
			t.Fatalf("watchlist_get shape: %+v", wl)
		}
	}
	// alert above 1 on BTCUSDT will fire (price >> 1); below 1 will not.
	_, _ = m.InvokeOp("flowalpha", "alert_add", map[string]any{"symbol": "BTCUSDT", "cond": "above", "price": 1.0}, "agent")
	if chk, err := m.InvokeOp("flowalpha", "alert_check", nil, "agent"); err == nil {
		if cm, _ := chk.(map[string]any); cm["triggered"] == nil {
			t.Fatalf("alert_check shape: %+v", chk)
		}
	} else if !netSkip(err) {
		t.Logf("alert_check skipped: %v", err)
	}
	_, _ = m.InvokeOp("flowalpha", "watchlist_remove", map[string]any{"symbol": "BTCUSDT"}, "agent")

	// Live trading is OWNER-GATED: live_order MUST refuse (no real money by default).
	if _, e := m.InvokeOp("flowalpha", "live_order", map[string]any{"side": "buy", "symbol": "BTCUSDT"}, "agent"); e == nil {
		t.Fatal("live_order must refuse when not owner-armed")
	}
	if ls, err := m.InvokeOp("flowalpha", "live_status", nil, "agent"); err == nil {
		if lm, _ := ls.(map[string]any); lm["live_enabled"] != false {
			t.Fatalf("live should be disabled by default: %+v", ls)
		}
	}

	// Market breadth: 24h ticker.
	if tk, err := m.InvokeOp("flowalpha", "get_ticker_24h", map[string]any{"symbol": "BTCUSDT"}, "agent"); err == nil {
		if tm, _ := tk.(map[string]any); tm["change_pct"] == nil {
			t.Fatalf("get_ticker_24h wrong shape: %+v", tk)
		}
	} else if !netSkip(err) {
		t.Fatalf("get_ticker_24h: %v", err)
	}

	// An unregistered op MUST be rejected (gate validation).
	if _, e := m.InvokeOp("flowalpha", "rm_rf", nil, "agent"); e == nil {
		t.Fatal("unregistered op must be rejected")
	}
}
