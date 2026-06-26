// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

type Letter struct {
	ID         int64  `json:"id"`
	LetterType string `json:"letter_type"`
	Recipient  string `json:"recipient"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	WrittenAt  string `json:"written_at"`
	SealedAt   string `json:"sealed_at,omitempty"`
}

var validLetterTypes = map[string]struct{}{
	"farewell":   {},
	"handover":   {},
	"reflection": {},
}

func (s *Store) WriteLetter(letterType, recipient, subject, body string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if letterType == "" || subject == "" || body == "" {
		return 0, fmt.Errorf("letter_type + subject + body required")
	}
	if _, ok := validLetterTypes[letterType]; !ok {
		return 0, fmt.Errorf("letter_type must be one of: farewell, handover, reflection (got %q)", letterType)
	}
	if recipient == "" {
		recipient = "all"
	}

	const (
		maxSubjectBytes = 4 * 1024
		maxBodyBytes    = 16 * 1024
	)
	if len(subject) > maxSubjectBytes {
		subject = subject[:maxSubjectBytes] + "…[truncated]"
	}
	if len(body) > maxBodyBytes {
		body = body[:maxBodyBytes] + "…[truncated]"
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO death_letter(letter_type, recipient, subject, body, written_at)
		 VALUES(?, ?, ?, ?, ?)`,
		letterType, recipient, subject, body, ts,
	)
	if err != nil {
		return 0, fmt.Errorf("insert letter: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) UpdateUnsealedLetter(id int64, subject, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if subject == "" || body == "" {
		return fmt.Errorf("subject + body required")
	}

	const (
		maxSubjectBytes = 4 * 1024
		maxBodyBytes    = 16 * 1024
	)
	if len(subject) > maxSubjectBytes {
		subject = subject[:maxSubjectBytes] + "…[truncated]"
	}
	if len(body) > maxBodyBytes {
		body = body[:maxBodyBytes] + "…[truncated]"
	}

	res, err := s.db.Exec(
		`UPDATE death_letter SET subject = ?, body = ?
		 WHERE id = ? AND sealed_at IS NULL AND deleted_at IS NULL`,
		subject, body, id,
	)
	if err != nil {
		return fmt.Errorf("update letter: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("letter id %d not found, sealed, or deleted (immutable)", id)
	}
	return nil
}

func (s *Store) SealLetter(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE death_letter SET sealed_at = ?
		 WHERE id = ? AND sealed_at IS NULL AND deleted_at IS NULL`,
		ts, id,
	)
	if err != nil {
		return fmt.Errorf("seal letter: %w", err)
	}

	return nil
}

func (s *Store) SealAllUnsealed() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE death_letter SET sealed_at = ?
		 WHERE sealed_at IS NULL AND deleted_at IS NULL`,
		ts,
	)
	if err != nil {
		return 0, fmt.Errorf("seal all unsealed: %w", err)
	}
	return res.RowsAffected()
}

func (s *Store) ReadLetters(recipient string, sealedOnly bool, limit int) ([]Letter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `SELECT id, letter_type, recipient, subject, body, written_at,
	                 COALESCE(sealed_at, '')
	          FROM death_letter WHERE deleted_at IS NULL`
	args := []any{}
	if recipient != "" {
		query += ` AND recipient = ?`
		args = append(args, recipient)
	}
	if sealedOnly {
		query += ` AND sealed_at IS NOT NULL`
	}
	query += ` ORDER BY written_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query letters: %w", err)
	}
	defer rows.Close()

	var out []Letter
	for rows.Next() {
		var l Letter
		if err := rows.Scan(&l.ID, &l.LetterType, &l.Recipient, &l.Subject,
			&l.Body, &l.WrittenAt, &l.SealedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) CountLetters(sealedOnly bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT COUNT(*) FROM death_letter WHERE deleted_at IS NULL`
	if sealedOnly {
		query += ` AND sealed_at IS NOT NULL`
	}

	var n int64
	if err := s.db.QueryRow(query).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
