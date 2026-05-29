// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 18 phase 1 scheduler support. Existing `schedules` table
//   di-extend via lazy ALTER + new `scheduler_runs` table via CREATE IF
//   NOT EXISTS. Locked agentdb.go ngga di-modify. Phase 2 (distributed
//   lock, multi-instance, advanced cron syntax L/W/#) → tambah file baru.
//
// scheduler.go — Section 18 phase 1: schedule extension + scheduler_runs
// audit log + accessors buat scheduler engine.

package agentdb

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SchedulerSchemaInit — lazy CREATE + ALTER. Idempotent — pakai info_schema
// helper via `pragma_table_info` untuk skip ALTER kalau column udah ada.
// Caller (scheduler engine) panggil first.
func (s *Store) SchedulerSchemaInit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// scheduler_runs table — sequential ID + status + audit.
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS scheduler_runs (
		  id            INTEGER PRIMARY KEY AUTOINCREMENT,
		  schedule_id   TEXT NOT NULL,
		  cron          TEXT NOT NULL,
		  task          TEXT NOT NULL,
		  started_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  finished_at   TEXT,
		  duration_ms   INTEGER NOT NULL DEFAULT 0,
		  status        TEXT NOT NULL DEFAULT 'pending',
		  result_text   TEXT NOT NULL DEFAULT '',
		  error_text    TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_scheduler_runs_schedule ON scheduler_runs(schedule_id);
		CREATE INDEX IF NOT EXISTS idx_scheduler_runs_status ON scheduler_runs(status);
		CREATE INDEX IF NOT EXISTS idx_scheduler_runs_started ON scheduler_runs(started_at DESC);
	`); err != nil {
		return fmt.Errorf("create scheduler_runs: %w", err)
	}
	// ALTER schedules: tambah 3 column kalau belum ada.
	if err := addColIfMissing(tx, "schedules", "last_run_at", "TEXT"); err != nil {
		return err
	}
	if err := addColIfMissing(tx, "schedules", "next_run_at", "TEXT"); err != nil {
		return err
	}
	if err := addColIfMissing(tx, "schedules", "enabled", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	return tx.Commit()
}

// addColIfMissing — query pragma_table_info, ALTER kalau column belum ada.
func addColIfMissing(tx *sql.Tx, table, col, typeSpec string) error {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return fmt.Errorf("pragma %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if serr := rows.Scan(&name); serr == nil {
			if strings.EqualFold(name, col) {
				return nil // already exists
			}
		}
	}
	if rerr := rows.Err(); rerr != nil {
		return rerr
	}
	rows.Close()
	stmt := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, col, typeSpec)
	if _, err := tx.Exec(stmt); err != nil {
		return fmt.Errorf("alter %s.%s: %w", table, col, err)
	}
	return nil
}

// ScheduleRow — minimal mirror dari `schedules` row dengan kolom scheduler
// runtime butuh. Field selain ini di-Load() biasa.
type ScheduleRow struct {
	ID         string  `json:"id"`
	Cron       string  `json:"cron"`
	Task       string  `json:"task"`
	Enabled    bool    `json:"enabled"`
	LastRunAt  *string `json:"last_run_at,omitempty"`
	NextRunAt  *string `json:"next_run_at,omitempty"`
	OrderIdx   int     `json:"order_idx"`
}

// ListSchedulesForRunner — SELECT * dari schedules + 3 column phase 2.
// Caller scheduler.tick pakai. Order by order_idx ASC.
func (s *Store) ListSchedulesForRunner() ([]ScheduleRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`
		SELECT id, cron, task, COALESCE(enabled, 1),
		       last_run_at, next_run_at, COALESCE(order_idx, 0)
		FROM schedules
		ORDER BY order_idx
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScheduleRow{}
	for rows.Next() {
		var r ScheduleRow
		var enabled int
		var lastRun, nextRun sql.NullString
		if serr := rows.Scan(&r.ID, &r.Cron, &r.Task, &enabled, &lastRun, &nextRun, &r.OrderIdx); serr != nil {
			return nil, serr
		}
		r.Enabled = enabled != 0
		if lastRun.Valid {
			v := lastRun.String
			r.LastRunAt = &v
		}
		if nextRun.Valid {
			v := nextRun.String
			r.NextRunAt = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdateScheduleRunTime — set last_run_at + next_run_at.
func (s *Store) UpdateScheduleRunTime(scheduleID, lastRunAt, nextRunAt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE schedules SET last_run_at = ?, next_run_at = ? WHERE id = ?`,
		lastRunAt, nextRunAt, scheduleID,
	)
	return err
}

// SchedulerRun — mirror scheduler_runs row.
type SchedulerRun struct {
	ID         int64  `json:"id"`
	ScheduleID string `json:"schedule_id"`
	Cron       string `json:"cron"`
	Task       string `json:"task"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	DurationMS int64  `json:"duration_ms"`
	Status     string `json:"status"`
	ResultText string `json:"result_text"`
	ErrorText  string `json:"error_text"`
}

// InsertSchedulerRun — append audit row. Return inserted ID.
func (s *Store) InsertSchedulerRun(run SchedulerRun) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(
		`INSERT INTO scheduler_runs
		   (schedule_id, cron, task, started_at, finished_at,
		    duration_ms, status, result_text, error_text)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ScheduleID, run.Cron, run.Task, run.StartedAt, run.FinishedAt,
		run.DurationMS, run.Status, run.ResultText, run.ErrorText,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSchedulerRuns — list audit rows for a schedule, paginated. Limit
// default 50, max 500.
func (s *Store) ListSchedulerRuns(scheduleID string, limit int) ([]SchedulerRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	query := `SELECT id, schedule_id, cron, task, started_at,
	                 COALESCE(finished_at, ''),
	                 duration_ms, status, result_text, error_text
	          FROM scheduler_runs`
	args := []any{}
	if scheduleID != "" {
		query += ` WHERE schedule_id = ?`
		args = append(args, scheduleID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SchedulerRun{}
	for rows.Next() {
		var r SchedulerRun
		if serr := rows.Scan(&r.ID, &r.ScheduleID, &r.Cron, &r.Task, &r.StartedAt,
			&r.FinishedAt, &r.DurationMS, &r.Status, &r.ResultText, &r.ErrorText); serr != nil {
			return nil, serr
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AbsTime utility — helper untuk format consistent RFC3339 timestamps.
func AbsTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
