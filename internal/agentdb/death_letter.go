// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 4 (Death letter) phase 1 DONE + adversarial-audit
//   passed (0 critical, important fix letter_type whitelist applied).
//   API stable: WriteLetter (whitelist enforced), UpdateUnsealedLetter
//   (immutable doctrine), SealLetter (idempotent one-way),
//   SealAllUnsealed (bulk for RemoveHandler integration), ReadLetters,
//   CountLetters. ⚠️ Immutable doctrine ENFORCED di app layer (WHERE
//   sealed_at IS NULL filter) — bukan strict crypto append-only.
//   Section 4 phase 2 (DownloadHandler zip integration) extend via
//   NEW function di file lain, JANGAN modify ini.
//
// death_letter.go — Section 4 roadmap: Death letter (legacy pesan).
//
// PURPOSE:
//   Saat warga di-retire (toggle off + remove), warga / owner tulis "death
//   letter" — pesan terakhir, value yang dia carry, instruksi buat penerus.
//   Ikut di .fwagent.zip download (future: DownloadHandler enhancement).
//
// Visi Mr.Dev: Flowork = rumah AI yang bisa hidup walau Mr.Dev ngga ada
// lagi. Death letter = continuity mechanism antar generasi warga.
//
// SEMANTIC:
//   - WriteLetter: insert/update body sebelum sealed. Idempotent kalau
//     unsealed (overwrite OK). Setelah sealed, refuse.
//   - SealLetter: one-way operation. Sekali sealed, body immutable.
//   - ReadLetters: filter optional recipient + sealedOnly. Anti
//     over-prompt: JANGAN auto-inject ke system prompt — sensitif legacy.
//
// ⚠️ OVER-PROMPT WARNING (standar section 11):
//   Death letter content cuma di-akses via API endpoint atau saat warga
//   handover. Tidak auto-inject ke runtime prompt.

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

// Letter — satu row di tabel `death_letter`.
type Letter struct {
	ID         int64  `json:"id"`
	LetterType string `json:"letter_type"` // 'farewell' | 'handover' | 'reflection'
	Recipient  string `json:"recipient"`   // 'all' | '<successor_agent_id>'
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	WrittenAt  string `json:"written_at"`
	SealedAt   string `json:"sealed_at,omitempty"` // empty = unsealed (mutable)
}

// validLetterTypes — whitelist enforcement per roadmap section 4 spec.
// Strict supaya analytics + future filtering konsisten. Caller kirim type
// di luar whitelist → reject. Audit Section 4 important fix.
var validLetterTypes = map[string]struct{}{
	"farewell":   {},
	"handover":   {},
	"reflection": {},
}

// WriteLetter — insert letter baru. Caller TIDAK boleh update existing letter
// via fn ini — pakai UpdateUnsealedLetter() untuk edit unsealed. Sealed
// letter ngga bisa di-modify.
//
// Subject + body hard-cap (4KB subject, 16KB body) anti-bloat.
// letter_type wajib di whitelist (`farewell`|`handover`|`reflection`).
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

// UpdateUnsealedLetter — overwrite body/subject letter yang BELUM di-seal.
// Refuse kalau letter udah sealed (immutable doctrine).
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

// SealLetter — one-way operation. Set sealed_at = now. Idempotent: kalau
// sudah sealed, return nil (no error, no change).
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
	// Tidak check RowsAffected — idempotent (sealed sudah, no-op OK).
	return nil
}

// SealAllUnsealed — bulk seal semua letter yang belum sealed. Dipanggil
// otomatis saat agent di-remove (RemoveHandler) — pastikan legacy
// preserved sebelum folder dihapus.
//
// Return count yang ke-seal.
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

// ReadLetters — paginated list. Filter optional: recipient, sealedOnly.
// Order: written_at DESC (terbaru dulu).
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

// CountLetters — count non-deleted. Optional sealedOnly.
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
