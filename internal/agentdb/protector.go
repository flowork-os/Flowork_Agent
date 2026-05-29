// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 24 phase 1 protector schema. Lazy CREATE. Phase 2
//   (rule revisions, version vector, escalation chain) → tambah file
//   baru, JANGAN modify ini.
//
// protector.go — Section 24 phase 1: protector_rules + protector_audit.

package agentdb

import (
	"fmt"
	"strings"
	"time"
)

// ProtectorRule mirrors row. Hardcoded source = immutable (DB delete
// no-op secara security karena baseline.go Go memory wins).
type ProtectorRule struct {
	ID        int64  `json:"id"`
	RuleType  string `json:"rule_type"`
	Pattern   string `json:"pattern"`
	Action    string `json:"action"`
	Source    string `json:"source"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// ProtectorAudit row.
type ProtectorAudit struct {
	ID         int64  `json:"id"`
	OccurredAt string `json:"occurred_at"`
	ToolName   string `json:"tool_name"`
	PatternHit string `json:"pattern_hit"`
	Decision   string `json:"decision"`
	ArgsHash   string `json:"args_hash"`
	Caller     string `json:"caller"`
}

func (s *Store) ensureProtectorSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS protector_rules (
		  id         INTEGER PRIMARY KEY AUTOINCREMENT,
		  rule_type  TEXT NOT NULL,
		  pattern    TEXT NOT NULL,
		  action     TEXT NOT NULL,
		  source     TEXT NOT NULL,
		  enabled    INTEGER NOT NULL DEFAULT 1,
		  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  UNIQUE(rule_type, pattern)
		);
		CREATE TABLE IF NOT EXISTS protector_audit (
		  id          INTEGER PRIMARY KEY AUTOINCREMENT,
		  occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  tool_name   TEXT NOT NULL,
		  pattern_hit TEXT NOT NULL,
		  decision    TEXT NOT NULL,
		  args_hash   TEXT NOT NULL,
		  caller      TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_protector_audit_time
		  ON protector_audit(occurred_at DESC);
	`)
	return err
}

// AddProtectorRule — INSERT. Source must be 'custom'. Caller (handler)
// reject kalau source='hardcoded' (anti DB tampering).
func (s *Store) AddProtectorRule(rule ProtectorRule) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return 0, err
	}
	if strings.EqualFold(rule.Source, "hardcoded") {
		return 0, fmt.Errorf("source 'hardcoded' reserved — use 'custom'")
	}
	if rule.Source == "" {
		rule.Source = "custom"
	}
	if rule.Action == "" {
		rule.Action = "block"
	}
	enabled := 1
	if !rule.Enabled {
		enabled = 0
	}
	res, err := s.db.Exec(
		`INSERT INTO protector_rules (rule_type, pattern, action, source, enabled)
		 VALUES (?, ?, ?, ?, ?)`,
		rule.RuleType, rule.Pattern, rule.Action, rule.Source, enabled,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListProtectorRules — return custom rows. Caller append hardcoded baseline
// via protector.Baseline() untuk fitur "see all".
func (s *Store) ListProtectorRules() ([]ProtectorRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, rule_type, pattern, action, source, enabled, created_at
		 FROM protector_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProtectorRule{}
	for rows.Next() {
		var r ProtectorRule
		var enabled int
		if serr := rows.Scan(&r.ID, &r.RuleType, &r.Pattern, &r.Action,
			&r.Source, &enabled, &r.CreatedAt); serr != nil {
			return nil, serr
		}
		r.Enabled = enabled != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteProtectorRule — reject kalau source=hardcoded di DB (additional
// safety beyond baseline.go immutable Go memory).
func (s *Store) DeleteProtectorRule(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return err
	}
	var source string
	err := s.db.QueryRow(`SELECT source FROM protector_rules WHERE id = ?`, id).Scan(&source)
	if err != nil {
		return fmt.Errorf("rule %d not found", id)
	}
	if strings.EqualFold(source, "hardcoded") {
		return fmt.Errorf("hardcoded rules cannot be deleted (immutable baseline)")
	}
	_, err = s.db.Exec(`DELETE FROM protector_rules WHERE id = ?`, id)
	return err
}

// ToggleProtectorRule.
func (s *Store) ToggleProtectorRule(id int64, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE protector_rules SET enabled = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rule %d not found", id)
	}
	return nil
}

// InsertProtectorAudit append.
func (s *Store) InsertProtectorAudit(a ProtectorAudit) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return 0, err
	}
	if a.OccurredAt == "" {
		a.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.Exec(
		`INSERT INTO protector_audit (occurred_at, tool_name, pattern_hit, decision, args_hash, caller)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.OccurredAt, a.ToolName, a.PatternHit, a.Decision, a.ArgsHash, a.Caller,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListProtectorAudit paginated.
func (s *Store) ListProtectorAudit(from, to string, limit int) ([]ProtectorAudit, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureProtectorSchema(); err != nil {
		return nil, err
	}
	query := `SELECT id, occurred_at, tool_name, pattern_hit, decision, args_hash, caller
	          FROM protector_audit WHERE 1=1`
	args := []any{}
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
	out := []ProtectorAudit{}
	for rows.Next() {
		var a ProtectorAudit
		if serr := rows.Scan(&a.ID, &a.OccurredAt, &a.ToolName, &a.PatternHit,
			&a.Decision, &a.ArgsHash, &a.Caller); serr != nil {
			return nil, serr
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
