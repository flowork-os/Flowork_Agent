// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package floworkdb

import (
	"database/sql"
	"fmt"
	"time"
)

type TaskSchedule struct {
	ID         int64  `json:"id"`
	Category   string `json:"category"`
	Subject    string `json:"subject"`
	Kind       string `json:"kind"`
	AtTime     string `json:"at_time"`
	EveryMin   int    `json:"every_min"`
	NotifyChat string `json:"notify_chat"`
	Enabled    bool   `json:"enabled"`
	LastRun    string `json:"last_run"`
	NextRun    string `json:"next_run"`
}

const schedTimeFmt = "2006-01-02 15:04:05"

func (s *Store) EnsureScheduleSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS task_schedules (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		category    TEXT NOT NULL,
		subject     TEXT NOT NULL,
		kind        TEXT NOT NULL DEFAULT 'daily',
		at_time     TEXT NOT NULL DEFAULT '09:00',
		every_min   INTEGER NOT NULL DEFAULT 0,
		notify_chat TEXT NOT NULL DEFAULT '',
		enabled     INTEGER NOT NULL DEFAULT 1,
		last_run    TEXT NOT NULL DEFAULT '',
		next_run    TEXT NOT NULL DEFAULT '',
		created_at  TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return err
}

func computeNextRun(sc TaskSchedule, now time.Time) time.Time {
	switch sc.Kind {
	case "every":
		m := sc.EveryMin
		if m < 1 {
			m = 60
		}
		return now.Add(time.Duration(m) * time.Minute)
	default:
		hh, mm := 9, 0
		fmt.Sscanf(sc.AtTime, "%d:%d", &hh, &mm)
		next := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}
}

func (s *Store) AddSchedule(sc TaskSchedule) (int64, error) {
	if err := s.EnsureScheduleSchema(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc.Kind == "" {
		sc.Kind = "daily"
	}
	next := computeNextRun(sc, time.Now()).Format(schedTimeFmt)
	res, err := s.db.Exec(
		`INSERT INTO task_schedules(category,subject,kind,at_time,every_min,notify_chat,enabled,next_run)
		 VALUES(?,?,?,?,?,?,1,?)`,
		sc.Category, sc.Subject, sc.Kind, sc.AtTime, sc.EveryMin, sc.NotifyChat, next)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) DeleteSchedule(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM task_schedules WHERE id=?`, id)
	return err
}

func (s *Store) ToggleSchedule(id int64, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	en := 0
	if enabled {
		en = 1
	}
	_, err := s.db.Exec(`UPDATE task_schedules SET enabled=? WHERE id=?`, en, id)
	return err
}

func scanSchedules(rows *sql.Rows) ([]TaskSchedule, error) {
	defer rows.Close()
	var out []TaskSchedule
	for rows.Next() {
		var sc TaskSchedule
		var en int
		if err := rows.Scan(&sc.ID, &sc.Category, &sc.Subject, &sc.Kind, &sc.AtTime,
			&sc.EveryMin, &sc.NotifyChat, &en, &sc.LastRun, &sc.NextRun); err != nil {
			return nil, err
		}
		sc.Enabled = en == 1
		out = append(out, sc)
	}
	return out, rows.Err()
}

const schedCols = `id,category,subject,kind,at_time,every_min,notify_chat,enabled,last_run,next_run`

func (s *Store) ListSchedules() ([]TaskSchedule, error) {
	if err := s.EnsureScheduleSchema(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT ` + schedCols + ` FROM task_schedules ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	return scanSchedules(rows)
}

func (s *Store) DueSchedules(now time.Time) ([]TaskSchedule, error) {
	if err := s.EnsureScheduleSchema(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(
		`SELECT `+schedCols+` FROM task_schedules
		 WHERE enabled=1 AND next_run<>'' AND next_run<=? ORDER BY id`,
		now.Format(schedTimeFmt))
	if err != nil {
		return nil, err
	}
	return scanSchedules(rows)
}

func (s *Store) MarkScheduleFired(sc TaskSchedule, now time.Time) error {
	next := computeNextRun(sc, now).Format(schedTimeFmt)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE task_schedules SET last_run=?, next_run=? WHERE id=?`,
		now.Format(schedTimeFmt), next, sc.ID)
	return err
}
