// Package db — task_events.go: B-3 + B-6 fix.
//
// Track event warga (memorize_brain, daily_reflection, roadmap_write) untuk
// enforcement karma deduction. Append-only (FQP-12), no UPDATE/DELETE.
//
// Schema:
//
//	task_events (
//	  id INTEGER PK,
//	  warga_id TEXT NOT NULL,    -- agents.name
//	  task_name TEXT,             -- workspace task ID, optional
//	  event_type TEXT,            -- 'memorize_brain' | 'daily_reflection' | 'roadmap_write'
//	  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
//	  metadata TEXT               -- JSON optional
//	)
//
// Helper:
//   - EnsureTaskEventsSchema: idempotent CREATE TABLE
//   - WriteTaskEvent: append-only insert
//   - LastEventAt: timestamp event terakhir per warga + type
//   - CountEventsLastNDays: count events dalam N hari (untuk auditor karma)
package db

import (
	"database/sql"
	"fmt"
	"time"
)

const TaskEventMemorize    = "memorize_brain"
const TaskEventReflection  = "daily_reflection"
const TaskEventRoadmap     = "roadmap_write"

// EnsureTaskEventsSchema idempotent CREATE TABLE + indexes.
// Boot caller pastikan schema ada sebelum WriteTaskEvent.
func EnsureTaskEventsSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			warga_id TEXT NOT NULL,
			task_name TEXT,
			event_type TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_events_warga ON task_events(warga_id, event_type, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_task_events_ts ON task_events(timestamp)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("ensure task_events schema: %w", err)
		}
	}
	return nil
}

// WriteTaskEvent append-only insert. Caller dari tool post-success path
// (memorize_brain.go, daily_reflection.go, roadmap_write.go).
//
// metadata optional JSON string — caller serialize sendiri.
func WriteTaskEvent(db *sql.DB, wargaID, taskName, eventType, metadata string) error {
	if wargaID == "" {
		return fmt.Errorf("task_event: warga_id wajib")
	}
	if eventType == "" {
		return fmt.Errorf("task_event: event_type wajib")
	}
	if err := EnsureTaskEventsSchema(db); err != nil {
		return err
	}
	_, err := db.Exec(
		`INSERT INTO task_events (warga_id, task_name, event_type, metadata) VALUES (?, ?, ?, ?)`,
		wargaID, taskName, eventType, metadata,
	)
	if err != nil {
		return fmt.Errorf("write task_event: %w", err)
	}
	return nil
}

// LastEventAt return timestamp event terakhir per warga + type.
// Empty time.Time + sql.ErrNoRows kalau belum pernah ada event.
func LastEventAt(db *sql.DB, wargaID, eventType string) (time.Time, error) {
	if err := EnsureTaskEventsSchema(db); err != nil {
		return time.Time{}, err
	}
	var ts string
	err := db.QueryRow(
		`SELECT timestamp FROM task_events WHERE warga_id = ? AND event_type = ? ORDER BY timestamp DESC LIMIT 1`,
		wargaID, eventType,
	).Scan(&ts)
	if err == sql.ErrNoRows {
		return time.Time{}, sql.ErrNoRows
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("query last event: %w", err)
	}
	t, parseErr := time.Parse("2006-01-02 15:04:05", ts)
	if parseErr != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", ts, parseErr)
	}
	return t, nil
}

// CountEventsLastNDays count event dalam N hari terakhir per warga + type.
// Auditor weekly query — kalau count < threshold, karma deduct.
func CountEventsLastNDays(db *sql.DB, wargaID, eventType string, days int) (int, error) {
	if err := EnsureTaskEventsSchema(db); err != nil {
		return 0, err
	}
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM task_events WHERE warga_id = ? AND event_type = ? AND timestamp >= ?`,
		wargaID, eventType, cutoff,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count events last %d days: %w", days, err)
	}
	return count, nil
}
