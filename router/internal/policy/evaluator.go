// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package policy

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type Engine struct {
	db       *sql.DB
	interval time.Duration
	stop     chan struct{}
}

func New(db *sql.DB) *Engine {
	return &Engine{
		db:       db,
		interval: 5 * time.Minute,
	}
}

func (e *Engine) Start(ctx context.Context) {
	e.stop = make(chan struct{})
	log.Printf("[policy] evaluator started — interval %s", e.interval)
	go e.loop(ctx)
}

func (e *Engine) Stop() {
	if e.stop != nil {
		close(e.stop)
	}
}

func (e *Engine) FireNow(ctx context.Context) (int, int) {
	return e.tick(ctx)
}

func (e *Engine) loop(ctx context.Context) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stop:
			return
		case <-timer.C:
			e.tick(ctx)
			timer.Reset(e.interval)
		}
	}
}

func (e *Engine) tick(ctx context.Context) (int, int) {
	rows, err := e.db.Query(
		`SELECT id, scope, scope_key, metric_key, budget_value, reset_period, warning_pct
		 FROM policy_budgets WHERE enabled = 1`)
	if err != nil {
		log.Printf("[policy] list budgets: %v", err)
		return 0, 0
	}
	defer rows.Close()

	type budget struct {
		ID          int64
		Scope       string
		ScopeKey    string
		MetricKey   string
		BudgetValue float64
		ResetPeriod string
		WarningPct  float64
	}
	var budgets []budget
	for rows.Next() {
		var b budget
		_ = rows.Scan(&b.ID, &b.Scope, &b.ScopeKey, &b.MetricKey, &b.BudgetValue, &b.ResetPeriod, &b.WarningPct)
		budgets = append(budgets, b)
	}

	evaluated := 0
	fired := 0
	for _, b := range budgets {
		evaluated++
		periodStart := periodStartFor(b.ResetPeriod)

		var spend float64
		q := `SELECT COALESCE(SUM(cost_usd), 0) FROM provider_call_log WHERE occurred_at >= ?`
		args := []any{periodStart.Format(time.RFC3339)}
		if b.ScopeKey != "" {
			q += ` AND caller = ?`
			args = append(args, b.ScopeKey)
		}
		_ = e.db.QueryRow(q, args...).Scan(&spend)

		action := ""
		if spend >= b.BudgetValue {
			action = "block"
		} else if spend >= b.BudgetValue*b.WarningPct {
			action = "warn"
		}
		if action == "" {
			continue
		}

		var existing int
		_ = e.db.QueryRow(
			`SELECT COUNT(*) FROM policy_violations
			 WHERE budget_id = ? AND fired_at >= ?`,
			b.ID, periodStart.Format(time.RFC3339)).Scan(&existing)
		if existing > 0 && action == "warn" {
			continue
		}

		_, _ = e.db.Exec(
			`INSERT INTO policy_violations (budget_id, actual_value, action_taken)
			 VALUES (?, ?, ?)`,
			b.ID, spend, action)
		log.Printf("[policy] budget_id=%d scope=%s/%s metric=%s spend=%.4f budget=%.4f → %s",
			b.ID, b.Scope, b.ScopeKey, b.MetricKey, spend, b.BudgetValue, action)
		fired++
	}
	if fired > 0 {
		log.Printf("[policy] tick evaluated=%d fired=%d", evaluated, fired)
	}
	return evaluated, fired
}

func periodStartFor(p string) time.Time {
	now := time.Now().UTC()
	switch p {
	case "weekly":

		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
	case "monthly":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "daily":
		fallthrough
	default:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
}

func (e *Engine) IsAllowed(ctx context.Context, caller string) (bool, string) {
	rows, err := e.db.Query(
		`SELECT id, budget_value, reset_period, warning_pct
		 FROM policy_budgets
		 WHERE enabled = 1 AND scope_key = ?
		   AND metric_key IN ('daily_usd','weekly_usd','monthly_usd','cost_usd')`,
		caller)
	if err != nil {
		return true, ""
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var budgetValue, warningPct float64
		var resetPeriod string
		if serr := rows.Scan(&id, &budgetValue, &resetPeriod, &warningPct); serr != nil {
			continue
		}
		periodStart := periodStartFor(resetPeriod)
		var spend float64
		_ = e.db.QueryRow(
			`SELECT COALESCE(SUM(cost_usd), 0) FROM provider_call_log
			 WHERE caller = ? AND occurred_at >= ?`,
			caller, periodStart.Format(time.RFC3339)).Scan(&spend)
		if spend >= budgetValue {
			return false, fmt.Sprintf("budget exceeded (id=%d spend=%.4f vs %.4f)",
				id, spend, budgetValue)
		}
	}
	return true, ""
}
