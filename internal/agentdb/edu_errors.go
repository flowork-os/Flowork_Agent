// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 9 (Educational error lookup phase 1) DONE.
//   API stable: UpsertEduError (atomic UPSERT, content+title cap),
//   LookupEduError (return zero+code on miss), ListEduErrors (category
//   filter, limit cap 500), CountEduErrors. Future PullEduErrors sync
//   dari Router → tambah file/function baru, JANGAN modify ini.
//
// edu_errors.go — Section 9 roadmap: Educational error lookup (lokal cache).
//
// PURPOSE:
//   Cache catalog error pendidikan (mirror schema dari Router future).
//   Warga lookup `code` → dapat explanation + remediation. Sync periodically
//   dari Router /api/edu-errors (defer Section 9 phase 2).
//
// SEMANTIC:
//   - UpsertEduError: PRIMARY KEY code → idempotent insert atau replace.
//   - LookupEduError(code): single read, return zero+code kalau ngga ada.
//   - ListEduErrors(category, limit): browse catalog.
//
// ⚠️ Anti over-prompt: lookup endpoint untuk diagnostic / decision log
// rationale. JANGAN bundle full catalog ke prompt.

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

// EduError — satu row di educational_errors_cache.
type EduError struct {
	Code        string `json:"code"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
	Remediation string `json:"remediation"`
	SyncedAt    string `json:"synced_at"`
}

// UpsertEduError — insert atau replace via PRIMARY KEY code. Hard cap
// 4KB explanation, 4KB remediation, 256 char title.
func (s *Store) UpsertEduError(e EduError) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.Code == "" || e.Category == "" || e.Title == "" || e.Explanation == "" {
		return fmt.Errorf("code + category + title + explanation required")
	}

	const (
		maxText  = 4 * 1024
		maxTitle = 256
	)
	if len(e.Explanation) > maxText {
		e.Explanation = e.Explanation[:maxText] + "…"
	}
	if len(e.Remediation) > maxText {
		e.Remediation = e.Remediation[:maxText] + "…"
	}
	if len(e.Title) > maxTitle {
		e.Title = e.Title[:maxTitle] + "…"
	}

	ts := time.Now().UTC().Format(time.RFC3339)

	// Atomic UPSERT via ON CONFLICT.
	_, err := s.db.Exec(
		`INSERT INTO educational_errors_cache(code, category, title, explanation, remediation, synced_at)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(code) DO UPDATE SET
		     category    = excluded.category,
		     title       = excluded.title,
		     explanation = excluded.explanation,
		     remediation = excluded.remediation,
		     synced_at   = excluded.synced_at,
		     deleted_at  = NULL`,
		e.Code, e.Category, e.Title, e.Explanation, e.Remediation, ts,
	)
	if err != nil {
		return fmt.Errorf("upsert edu error: %w", err)
	}
	return nil
}

// LookupEduError — single by code. Return zero EduError + code set kalau
// ngga ada (caller check Title == "").
func (s *Store) LookupEduError(code string) (EduError, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if code == "" {
		return EduError{}, fmt.Errorf("code required")
	}

	var e EduError
	err := s.db.QueryRow(
		`SELECT code, category, title, explanation, remediation, synced_at
		 FROM educational_errors_cache WHERE code = ? AND deleted_at IS NULL`,
		code,
	).Scan(&e.Code, &e.Category, &e.Title, &e.Explanation, &e.Remediation, &e.SyncedAt)
	if err == sql.ErrNoRows {
		return EduError{Code: code}, nil
	}
	if err != nil {
		return EduError{}, fmt.Errorf("lookup edu error: %w", err)
	}
	return e, nil
}

// ListEduErrors — paginated. Filter optional category. Order: synced_at DESC.
// Default 50, max 500.
func (s *Store) ListEduErrors(category string, limit int) ([]EduError, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `SELECT code, category, title, explanation, remediation, synced_at
	          FROM educational_errors_cache WHERE deleted_at IS NULL`
	args := []any{}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	query += ` ORDER BY synced_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edu errors: %w", err)
	}
	defer rows.Close()

	var out []EduError
	for rows.Next() {
		var e EduError
		if err := rows.Scan(&e.Code, &e.Category, &e.Title, &e.Explanation,
			&e.Remediation, &e.SyncedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountEduErrors — total non-deleted.
func (s *Store) CountEduErrors() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int64
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM educational_errors_cache WHERE deleted_at IS NULL`,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
