// Package ingestor — mistakes_journal.go
// Mistakes Journal — learn from errors via spaced repetition (Anki-style).
//
// KEPUTUSAN Opus-3 §2.5 + Opus-2 APPROVE:
// - Track every mistake (Ayah correction, self-eval fail, user feedback)
// - Dream daemon weekly: pick unresolved → re-ask → assert correct
// - Mistake REPEATED → karma penalty + importance boost to correction
// - 5x consecutive pass → mark resolved (tetap di journal, tapi skip retest)
//
// FQP-12: append-only (never delete mistakes, only resolve).
package ingestor

import (
	"database/sql"
	"time"
)

// Mistake represents one entry in the mistakes_journal.
type Mistake struct {
	ID              int       `json:"id"`
	Query           string    `json:"query"`
	WrongAnswer     string    `json:"wrong_answer"`
	CorrectAnswer   string    `json:"correct_answer"`
	Source          string    `json:"source"` // ayah_correction | self_eval | user_feedback
	DetectedAt      time.Time `json:"detected_at"`
	Domain          string    `json:"domain"` // whitehat | trading | indonesian | general
	Pattern         string    `json:"pattern,omitempty"`
	RetestAt        time.Time `json:"retest_at"`
	RetestPassCount int       `json:"retest_pass_count"`
	RetestFailCount int       `json:"retest_fail_count"`
	Resolved        bool      `json:"resolved"`
}

// EnsureMistakesJournalSchema creates the mistakes_journal table.
func EnsureMistakesJournalSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS mistakes_journal (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query TEXT NOT NULL,
		wrong_answer TEXT NOT NULL,
		correct_answer TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'self_eval',
		detected_at TEXT DEFAULT CURRENT_TIMESTAMP,
		domain TEXT NOT NULL DEFAULT 'general',
		pattern TEXT DEFAULT '',
		retest_at TEXT DEFAULT '',
		retest_pass_count INTEGER DEFAULT 0,
		retest_fail_count INTEGER DEFAULT 0,
		resolved INTEGER DEFAULT 0
	)`)
	if err != nil {
		return err
	}

	// Index for dream daemon weekly scan
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_mistakes_unresolved
		ON mistakes_journal(resolved, retest_at)
		WHERE resolved = 0`)

	return nil
}

// EnsurePromptInjectionLogSchema creates the prompt_injection_log table.
func EnsurePromptInjectionLogSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS prompt_injection_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT DEFAULT CURRENT_TIMESTAMP,
		session_id TEXT DEFAULT '',
		caller_id TEXT DEFAULT '',
		raw_input TEXT NOT NULL,
		detected_pattern TEXT NOT NULL,
		action_taken TEXT DEFAULT 'blocked',
		karma_penalty REAL DEFAULT 0.0
	)`)
	return err
}

// RecordMistake inserts a new mistake entry.
// Retest schedule: 1 day → 1 week → 1 month → 3 months → 1 year (spaced repetition).
func RecordMistake(db *sql.DB, query, wrongAnswer, correctAnswer, source, domain string) error {
	retestAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	_, err := db.Exec(`INSERT INTO mistakes_journal
		(query, wrong_answer, correct_answer, source, domain, retest_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		query, wrongAnswer, correctAnswer, source, domain, retestAt)
	return err
}

// GetUnresolvedMistakes returns mistakes due for retest.
func GetUnresolvedMistakes(db *sql.DB, limit int) ([]Mistake, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := db.Query(`SELECT id, query, wrong_answer, correct_answer, source,
		domain, pattern, retest_at, retest_pass_count, retest_fail_count
		FROM mistakes_journal
		WHERE resolved = 0 AND retest_at <= ?
		ORDER BY retest_fail_count DESC, retest_at ASC
		LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mistakes []Mistake
	for rows.Next() {
		var m Mistake
		var retestAt string
		err := rows.Scan(&m.ID, &m.Query, &m.WrongAnswer, &m.CorrectAnswer,
			&m.Source, &m.Domain, &m.Pattern, &retestAt,
			&m.RetestPassCount, &m.RetestFailCount)
		if err != nil {
			continue
		}
		m.RetestAt, _ = time.Parse(time.RFC3339, retestAt)
		mistakes = append(mistakes, m)
	}
	// Sprint 3.5d (BUG-C15 fix): rows.Err() check
	if err := rows.Err(); err != nil {
		return mistakes, err
	}
	return mistakes, nil
}

// RecordRetestResult updates a mistake after retest.
// Pass → increment pass_count, schedule next retest (spaced repetition).
// Fail → increment fail_count, schedule retest in 1 day (retry soon).
// 5x consecutive pass → mark resolved.
func RecordRetestResult(db *sql.DB, mistakeID int, passed bool) error {
	if passed {
		// Check current pass count
		var passCount int
		err := db.QueryRow(`SELECT retest_pass_count FROM mistakes_journal WHERE id = ?`, mistakeID).Scan(&passCount)
		if err != nil {
			return err
		}

		newPassCount := passCount + 1
		resolved := 0
		var nextRetest time.Time

		if newPassCount >= 5 {
			// 5x consecutive pass → resolved
			resolved = 1
			nextRetest = time.Now().Add(365 * 24 * time.Hour) // far future
		} else {
			// Spaced repetition schedule
			intervals := []time.Duration{
				24 * time.Hour,       // after 1st pass: retest in 1 day
				7 * 24 * time.Hour,   // after 2nd pass: retest in 1 week
				30 * 24 * time.Hour,  // after 3rd pass: retest in 1 month
				90 * 24 * time.Hour,  // after 4th pass: retest in 3 months
			}
			idx := newPassCount - 1
			if idx >= len(intervals) {
				idx = len(intervals) - 1
			}
			nextRetest = time.Now().Add(intervals[idx])
		}

		_, err = db.Exec(`UPDATE mistakes_journal
			SET retest_pass_count = ?, retest_fail_count = 0, resolved = ?, retest_at = ?
			WHERE id = ?`,
			newPassCount, resolved, nextRetest.UTC().Format(time.RFC3339), mistakeID)
		return err
	}

	// Failed retest → reset pass count, retry in 1 day
	nextRetest := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	_, err := db.Exec(`UPDATE mistakes_journal
		SET retest_fail_count = retest_fail_count + 1, retest_pass_count = 0, retest_at = ?
		WHERE id = ?`, nextRetest, mistakeID)
	return err
}

// RecordPromptInjection logs a detected prompt injection attempt.
func RecordPromptInjection(db *sql.DB, sessionID, callerID, rawInput, pattern, action string, karmaPenalty float64) error {
	_, err := db.Exec(`INSERT INTO prompt_injection_log
		(session_id, caller_id, raw_input, detected_pattern, action_taken, karma_penalty)
		VALUES (?, ?, ?, ?, ?, ?)`,
		sessionID, callerID, rawInput, pattern, action, karmaPenalty)
	return err
}
