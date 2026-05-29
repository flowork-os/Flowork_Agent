// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 26 phase 1 — audit_log + watchdog_alerts schema. Append-
//   only enforced via Go API (no Update/Delete methods). Phase 2 (trigger
//   block UPDATE/DELETE, hash-chain rows, real-time watchdog daemon) →
//   tambah file baru.
//
// audit.go — Section 26 phase 1: audit log + watchdog alerts.

package agentdb

import (
	"fmt"
	"time"
)

const (
	EventToolCall        = "tool_call"
	EventProtectorBlock  = "protector_block"
	EventScannerFinding  = "scanner_finding"
	EventConfigChange    = "config_change"
)

const (
	AuditSevInfo     = "info"
	AuditSevWarning  = "warning"
	AuditSevError    = "error"
	AuditSevCritical = "critical"
)

type AuditEntry struct {
	ID         int64  `json:"id"`
	EventType  string `json:"event_type"`
	Severity   string `json:"severity"`
	Actor      string `json:"actor"`
	DetailJSON string `json:"detail_json"`
	OccurredAt string `json:"occurred_at"`
}

type WatchdogAlert struct {
	ID          int64  `json:"id"`
	RuleID      string `json:"rule_id"`
	FiredAt     string `json:"fired_at"`
	ContextJSON string `json:"context_json"`
	Notified    bool   `json:"notified"`
}

func (s *Store) ensureAuditSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_log (
		  id          INTEGER PRIMARY KEY AUTOINCREMENT,
		  event_type  TEXT NOT NULL,
		  severity    TEXT NOT NULL,
		  actor       TEXT NOT NULL DEFAULT '',
		  detail_json TEXT NOT NULL DEFAULT '{}',
		  occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_audit_event ON audit_log(event_type);
		CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_log(occurred_at DESC);

		CREATE TABLE IF NOT EXISTS watchdog_alerts (
		  id           INTEGER PRIMARY KEY AUTOINCREMENT,
		  rule_id      TEXT NOT NULL,
		  fired_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  context_json TEXT NOT NULL DEFAULT '{}',
		  notified     INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_watchdog_fired ON watchdog_alerts(fired_at DESC);
	`)
	return err
}

// AppendAudit — append-only. NO Update/Delete API exposed (immutability).
func (s *Store) AppendAudit(e AuditEntry) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAuditSchema(); err != nil {
		return 0, err
	}
	if e.EventType == "" {
		return 0, fmt.Errorf("event_type required")
	}
	if e.Severity == "" {
		e.Severity = AuditSevInfo
	}
	if e.OccurredAt == "" {
		e.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if e.DetailJSON == "" {
		e.DetailJSON = "{}"
	}
	res, err := s.db.Exec(
		`INSERT INTO audit_log (event_type, severity, actor, detail_json, occurred_at)
		 VALUES (?, ?, ?, ?, ?)`,
		e.EventType, e.Severity, e.Actor, e.DetailJSON, e.OccurredAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListAudit(eventType, from, to string, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAuditSchema(); err != nil {
		return nil, err
	}
	query := `SELECT id, event_type, severity, actor, detail_json, occurred_at
	          FROM audit_log WHERE 1=1`
	args := []any{}
	if eventType != "" {
		query += ` AND event_type = ?`
		args = append(args, eventType)
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
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if serr := rows.Scan(&e.ID, &e.EventType, &e.Severity, &e.Actor,
			&e.DetailJSON, &e.OccurredAt); serr != nil {
			return nil, serr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountAuditInWindow — buat watchdog rule eval (mis. "≥10 protector_block
// in 60s"). Return count of rows yang match event_type + occurred_at >= sinceISO.
func (s *Store) CountAuditInWindow(eventType, sinceISO string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAuditSchema(); err != nil {
		return 0, err
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM audit_log
		 WHERE event_type = ? AND occurred_at >= ?`,
		eventType, sinceISO).Scan(&n)
	return n, err
}

func (s *Store) InsertWatchdogAlert(a WatchdogAlert) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAuditSchema(); err != nil {
		return 0, err
	}
	if a.FiredAt == "" {
		a.FiredAt = time.Now().UTC().Format(time.RFC3339)
	}
	notified := 0
	if a.Notified {
		notified = 1
	}
	res, err := s.db.Exec(
		`INSERT INTO watchdog_alerts (rule_id, fired_at, context_json, notified)
		 VALUES (?, ?, ?, ?)`,
		a.RuleID, a.FiredAt, a.ContextJSON, notified)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListWatchdogAlerts(limit int) ([]WatchdogAlert, error) {
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureAuditSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT id, rule_id, fired_at, context_json, notified
		 FROM watchdog_alerts ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchdogAlert{}
	for rows.Next() {
		var a WatchdogAlert
		var notified int
		if serr := rows.Scan(&a.ID, &a.RuleID, &a.FiredAt, &a.ContextJSON, &notified); serr != nil {
			return nil, serr
		}
		a.Notified = notified != 0
		out = append(out, a)
	}
	return out, rows.Err()
}
