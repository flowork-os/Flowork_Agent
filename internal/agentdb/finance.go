// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 23 phase 1 finance ledger schema + accessors. Lazy
//   CREATE. Phase 2 (currency-aware aggregate, FX rate, multi-account)
//   → tambah file baru.
//
// finance.go — Section 23 phase 1: ledger row + budget config + summary.

package agentdb

import (
	"fmt"
	"strings"
	"time"
)

// FinanceLedger — mirror finance_ledger row.
type FinanceLedger struct {
	ID            int64   `json:"id"`
	OccurredAt    string  `json:"occurred_at"`
	Category      string  `json:"category"`
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	CostUSD       float64 `json:"cost_usd"`
	MetadataJSON  string  `json:"metadata_json"`
}

// FinanceBudget — mirror finance_budgets row.
type FinanceBudget struct {
	MetricKey    string  `json:"metric_key"`
	BudgetValue  float64 `json:"budget_value"`
	WarningAtPct float64 `json:"warning_at_pct"`
	Enabled      bool    `json:"enabled"`
}

func (s *Store) ensureFinanceSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS finance_ledger (
		  id            INTEGER PRIMARY KEY AUTOINCREMENT,
		  occurred_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  category      TEXT NOT NULL,
		  provider      TEXT NOT NULL DEFAULT '',
		  model         TEXT NOT NULL DEFAULT '',
		  input_tokens  INTEGER NOT NULL DEFAULT 0,
		  output_tokens INTEGER NOT NULL DEFAULT 0,
		  cost_usd      REAL NOT NULL DEFAULT 0,
		  metadata_json TEXT NOT NULL DEFAULT '{}'
		);
		CREATE INDEX IF NOT EXISTS idx_finance_ledger_time
		  ON finance_ledger(occurred_at DESC);
		CREATE INDEX IF NOT EXISTS idx_finance_ledger_category
		  ON finance_ledger(category);
		CREATE TABLE IF NOT EXISTS finance_budgets (
		  metric_key     TEXT PRIMARY KEY,
		  budget_value   REAL NOT NULL,
		  warning_at_pct REAL NOT NULL DEFAULT 0.8,
		  enabled        INTEGER NOT NULL DEFAULT 1
		);
	`)
	return err
}

// AddLedger append row, return ID.
func (s *Store) AddLedger(l FinanceLedger) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureFinanceSchema(); err != nil {
		return 0, err
	}
	l.Category = strings.TrimSpace(l.Category)
	if l.Category == "" {
		return 0, fmt.Errorf("category required")
	}
	if l.OccurredAt == "" {
		l.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if l.MetadataJSON == "" {
		l.MetadataJSON = "{}"
	}
	res, err := s.db.Exec(
		`INSERT INTO finance_ledger
		   (occurred_at, category, provider, model,
		    input_tokens, output_tokens, cost_usd, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		l.OccurredAt, l.Category, l.Provider, l.Model,
		l.InputTokens, l.OutputTokens, l.CostUSD, l.MetadataJSON,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListLedger paginated, filter by category + time range.
func (s *Store) ListLedger(category, from, to string, limit int) ([]FinanceLedger, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureFinanceSchema(); err != nil {
		return nil, err
	}
	query := `SELECT id, occurred_at, category, provider, model,
	                 input_tokens, output_tokens, cost_usd, metadata_json
	          FROM finance_ledger WHERE 1=1`
	args := []any{}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	if from != "" {
		query += ` AND occurred_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND occurred_at <= ?`
		args = append(args, to)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinanceLedger{}
	for rows.Next() {
		var l FinanceLedger
		if serr := rows.Scan(&l.ID, &l.OccurredAt, &l.Category, &l.Provider, &l.Model,
			&l.InputTokens, &l.OutputTokens, &l.CostUSD, &l.MetadataJSON); serr != nil {
			return nil, serr
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LedgerSummary — aggregate cost per category dalam range.
type LedgerSummary struct {
	Category    string  `json:"category"`
	CostUSD     float64 `json:"cost_usd"`
	CallCount   int     `json:"call_count"`
	InputToks   int64   `json:"input_tokens"`
	OutputToks  int64   `json:"output_tokens"`
}

func (s *Store) SummaryLedger(from, to string) ([]LedgerSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureFinanceSchema(); err != nil {
		return nil, err
	}
	query := `SELECT category,
	                 COALESCE(SUM(cost_usd), 0),
	                 COUNT(*),
	                 COALESCE(SUM(input_tokens), 0),
	                 COALESCE(SUM(output_tokens), 0)
	          FROM finance_ledger WHERE 1=1`
	args := []any{}
	if from != "" {
		query += ` AND occurred_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		query += ` AND occurred_at <= ?`
		args = append(args, to)
	}
	query += ` GROUP BY category ORDER BY 2 DESC`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LedgerSummary{}
	for rows.Next() {
		var sm LedgerSummary
		if serr := rows.Scan(&sm.Category, &sm.CostUSD, &sm.CallCount, &sm.InputToks, &sm.OutputToks); serr != nil {
			return nil, serr
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// SetBudget upsert.
func (s *Store) SetBudget(b FinanceBudget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureFinanceSchema(); err != nil {
		return err
	}
	enabled := 1
	if !b.Enabled {
		enabled = 0
	}
	if b.WarningAtPct <= 0 {
		b.WarningAtPct = 0.8
	}
	_, err := s.db.Exec(
		`INSERT INTO finance_budgets (metric_key, budget_value, warning_at_pct, enabled)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(metric_key) DO UPDATE SET
		   budget_value = excluded.budget_value,
		   warning_at_pct = excluded.warning_at_pct,
		   enabled = excluded.enabled`,
		b.MetricKey, b.BudgetValue, b.WarningAtPct, enabled,
	)
	return err
}

// ListBudgets all.
func (s *Store) ListBudgets() ([]FinanceBudget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureFinanceSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT metric_key, budget_value, warning_at_pct, enabled FROM finance_budgets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinanceBudget{}
	for rows.Next() {
		var b FinanceBudget
		var enabled int
		if serr := rows.Scan(&b.MetricKey, &b.BudgetValue, &b.WarningAtPct, &enabled); serr != nil {
			return nil, serr
		}
		b.Enabled = enabled != 0
		out = append(out, b)
	}
	return out, rows.Err()
}
