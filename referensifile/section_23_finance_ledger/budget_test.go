package finance

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestBudgetGuard_Defaults(t *testing.T) {
	// Unset env untuk test defaults
	os.Unsetenv("FLOWORK_BUDGET_DAILY_USD")
	os.Unsetenv("FLOWORK_BUDGET_PER_TASK_USD")
	os.Unsetenv("FLOWORK_BUDGET_FREE_FALLBACK")

	g := NewBudgetGuard()
	if g.DailyCapUSD != DefaultBudgetDailyUSD {
		t.Errorf("default daily: got %f, want %f", g.DailyCapUSD, DefaultBudgetDailyUSD)
	}
	if g.PerTaskCapUSD != DefaultBudgetPerTaskUSD {
		t.Errorf("default per-task: got %f, want %f", g.PerTaskCapUSD, DefaultBudgetPerTaskUSD)
	}
	if !g.FreeFallback {
		t.Error("FreeFallback should default true")
	}
}

func TestBudgetGuard_EnvOverride(t *testing.T) {
	t.Setenv("FLOWORK_BUDGET_DAILY_USD", "2.00")
	t.Setenv("FLOWORK_BUDGET_PER_TASK_USD", "0.10")
	t.Setenv("FLOWORK_BUDGET_FREE_FALLBACK", "0")

	g := NewBudgetGuard()
	if g.DailyCapUSD != 2.00 {
		t.Errorf("daily override: got %f, want 2.00", g.DailyCapUSD)
	}
	if g.PerTaskCapUSD != 0.10 {
		t.Errorf("per-task override: got %f, want 0.10", g.PerTaskCapUSD)
	}
	if g.FreeFallback {
		t.Error("FreeFallback should be disabled when env='0'")
	}
}

func TestBudgetGuard_WouldExceed_PerTaskCap(t *testing.T) {
	g := &BudgetGuard{DailyCapUSD: 10.0, PerTaskCapUSD: 0.05}
	// Single call above per-task cap → exceed regardless of daily
	if !g.WouldExceed(0.10) {
		t.Error("expected exceed when estimate > per-task cap")
	}
	// Below per-task cap + no usage → OK
	if g.WouldExceed(0.03) {
		t.Error("expected OK when estimate < per-task cap and no usage")
	}
}

func TestBudgetGuard_WouldExceed_DailyCap(t *testing.T) {
	g := &BudgetGuard{DailyCapUSD: 0.50, PerTaskCapUSD: 0.05}
	g.usedToday = 0.48 // near cap
	// 0.03 brings total to 0.51 > 0.50 → exceed
	if !g.WouldExceed(0.03) {
		t.Error("expected exceed when used+estimate > daily cap")
	}
	// 0.01 brings total to 0.49 < 0.50 → OK
	if g.WouldExceed(0.01) {
		t.Error("expected OK when used+estimate < daily cap")
	}
}

func TestBudgetGuard_CheckBudget_Error(t *testing.T) {
	// Force poll failure by unsetting key
	t.Setenv("OPENROUTER_API_KEY", "")
	g := &BudgetGuard{DailyCapUSD: 0.50, PerTaskCapUSD: 0.05, usedToday: 0.48}
	err := g.CheckBudget(context.Background(), 0.10)
	if err == nil {
		t.Fatal("expected error when per-task exceeded")
	}
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("expected errors.Is(err, ErrBudgetExceeded), got %v", err)
	}
}

func TestBudgetGuard_Record_Accumulates(t *testing.T) {
	g := &BudgetGuard{DailyCapUSD: 1.0, PerTaskCapUSD: 0.5}
	g.Record(0.10)
	g.Record(0.20)
	// Float precision tolerance — 0.1+0.2 != exactly 0.3 in IEEE-754.
	const epsilon = 1e-9
	used := g.Used()
	if used < 0.30-epsilon || used > 0.30+epsilon {
		t.Errorf("Used() after 2 records: got %f, want ~0.30", used)
	}
}

func TestBudgetGuard_GetStatus(t *testing.T) {
	g := &BudgetGuard{DailyCapUSD: 1.0, PerTaskCapUSD: 0.05, usedToday: 0.25}
	s := g.GetStatus()
	if s.DailyCap != 1.0 {
		t.Errorf("DailyCap: got %f", s.DailyCap)
	}
	if s.UsedToday != 0.25 {
		t.Errorf("UsedToday: got %f", s.UsedToday)
	}
	if s.Remaining != 0.75 {
		t.Errorf("Remaining: got %f want 0.75", s.Remaining)
	}
	if s.PercentUsed != 25.0 {
		t.Errorf("PercentUsed: got %f want 25.0", s.PercentUsed)
	}
}

func TestBudgetGuard_Negative_EstimateClamped(t *testing.T) {
	g := &BudgetGuard{DailyCapUSD: 0.50, PerTaskCapUSD: 0.05}
	// Negative estimate should be treated as 0, not cause exceed
	if g.WouldExceed(-1.0) {
		t.Error("negative estimate should be clamped to 0, not exceed")
	}
}
