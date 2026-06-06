// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 2 (Mistakes journal phase 1) DONE + adversarial-audit
//   passed (2 critical fixed: soft-delete+UNIQUE trap, addedNew logic).
//   API stable: AddMistake (atomic tx, undelete-on-conflict, return
//   accurate addedNew flag), ListMistakes (tier filter), Count, Prune.
//   Section 7 (Promotion ke router antibody) extend via NEW function di
//   file lain — JANGAN ubah ini tanpa approval.
//
// mistakes.go — Section 2 roadmap: Mistakes journal per-warga.
//
// PURPOSE:
//   Catat lesson / kesalahan personal warga. Tier `raw` di lokal. Setelah
//   validasi (hit_count tinggi → frequent pattern, atau manual review),
//   di-promote ke router brain sebagai antibody global (defer ke section 7
//   cross-tubuh sync). Tujuan: belajar dari mistake, share ke warga lain.
//
// ⚠️ OVER-PROMPT WARNING (per standar_ai_agent.md section 11):
//   JANGAN auto-inject SEMUA mistakes ke persona. Pakai pattern: top-3
//   recent only kalau context match. Sisanya retrieved via `brain_search
//   type=antibody` tool.
//
// Phase 1 scope (sekarang):
//   - AddMistake: UNIQUE(category,title) upsert — insert atau increment hit_count
//   - ListMistakes(tier, limit): filter optional
//   - CountMistakes(tier)
//   - PruneMistakes: soft-delete tier='raw' lama
//
// Defer:
//   - PromoteMistake → router brain antibody (section 7)
//   - host capability log_mistake (sampai ada use case real)

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

// Mistake — satu row di tabel `mistakes_local`.
type Mistake struct {
	ID            int64  `json:"id"`
	Category      string `json:"category"`        // 'logic' | 'safety' | 'performance' | dst
	Title         string `json:"title"`
	Content       string `json:"content"`
	ContextOrigin string `json:"context_origin"`  // interaction_id, decision_id, free-form
	Tier          string `json:"tier"`            // 'raw' | 'reviewed' | 'promoted'
	HitCount      int64  `json:"hit_count"`
	LastHitAt     string `json:"last_hit_at"`
	CreatedAt     string `json:"created_at"`
	PromotedAt    string `json:"promoted_at,omitempty"`
	PromotedToID  string `json:"promoted_to_id,omitempty"`
}

// AddMistake — idempotent insert via UNIQUE(category, title). Semantik:
//
//   - Row baru (category+title belum ada) → INSERT, return (newID, true, nil)
//   - Row existing live → UPDATE content + increment hit_count + bump last_hit_at,
//     return (existingID, false, nil)
//   - Row existing tapi soft-deleted → UNDELETE (deleted_at=NULL) + UPDATE
//     content + increment hit_count, return (existingID, false, nil).
//     Rationale: pattern muncul lagi = re-validate, hit_count terus akumulasi.
//     Loss provenance soft-delete acceptable — kalau perlu audit, log decision
//     separate.
//
// Atomic via transaction. SELECT-then-INSERT-or-UPDATE clearer dari ON CONFLICT
// dengan WHERE filter (audit Section 2 finding: bug ON CONFLICT DO UPDATE
// dengan `WHERE deleted_at IS NULL` → silent no-op + caller error "no rows"
// kalau row sebelumnya soft-deleted).
//
// Content hard-cap 4KB + title 256 char anti-bloat.
func (s *Store) AddMistake(category, title, content, contextOrigin string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if category == "" || title == "" || content == "" {
		return 0, false, fmt.Errorf("category + title + content required")
	}

	const (
		maxContentBytes = 4 * 1024
		maxTitleBytes   = 256
	)
	if len(content) > maxContentBytes {
		content = content[:maxContentBytes] + "…[truncated]"
	}
	if len(title) > maxTitleBytes {
		title = title[:maxTitleBytes] + "…"
	}

	ts := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Lookup existing row regardless of deleted_at (UNIQUE constraint cover
	// live + soft-deleted — biar logic ngga collide).
	var existingID int64
	err = tx.QueryRow(
		`SELECT id FROM mistakes_local WHERE category = ? AND title = ?`,
		category, title,
	).Scan(&existingID)

	switch {
	case err == sql.ErrNoRows:
		// Fresh INSERT.
		res, ierr := tx.Exec(
			`INSERT INTO mistakes_local(category, title, content, context_origin, last_hit_at, created_at)
			 VALUES(?, ?, ?, ?, ?, ?)`,
			category, title, content, contextOrigin, ts, ts,
		)
		if ierr != nil {
			return 0, false, fmt.Errorf("insert mistake: %w", ierr)
		}
		newID, _ := res.LastInsertId()
		if cerr := tx.Commit(); cerr != nil {
			return 0, false, fmt.Errorf("commit insert: %w", cerr)
		}
		tx = nil
		return newID, true, nil

	case err != nil:
		return 0, false, fmt.Errorf("lookup mistake: %w", err)

	default:
		// UPDATE existing (auto-undelete kalau soft-deleted). Tier ngga
		// di-rewind — kalau row sebelumnya 'reviewed' / 'promoted', stays.
		_, uerr := tx.Exec(
			`UPDATE mistakes_local SET
			    content     = ?,
			    last_hit_at = ?,
			    hit_count   = hit_count + 1,
			    deleted_at  = NULL,
			    deleted_by  = NULL
			 WHERE id = ?`,
			content, ts, existingID,
		)
		if uerr != nil {
			return 0, false, fmt.Errorf("upsert mistake: %w", uerr)
		}
		if cerr := tx.Commit(); cerr != nil {
			return 0, false, fmt.Errorf("commit upsert: %w", cerr)
		}
		tx = nil
		return existingID, false, nil
	}
}

// ListMistakes — paginated list. Filter optional: tier.
// Order: last_hit_at DESC (terbaru dulu — supaya top-K retrieval relevant).
// Limit default 50, max 500.
func (s *Store) ListMistakes(tier string, limit int) ([]Mistake, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}

	query := `SELECT id, category, title, content, context_origin, tier, hit_count,
	                 last_hit_at, created_at,
	                 COALESCE(promoted_at, ''), COALESCE(promoted_to_id, '')
	          FROM mistakes_local WHERE deleted_at IS NULL`
	args := []any{}
	if tier != "" {
		query += ` AND tier = ?`
		args = append(args, tier)
	}
	query += ` ORDER BY last_hit_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query mistakes: %w", err)
	}
	defer rows.Close()

	var out []Mistake
	for rows.Next() {
		var m Mistake
		if err := rows.Scan(&m.ID, &m.Category, &m.Title, &m.Content,
			&m.ContextOrigin, &m.Tier, &m.HitCount,
			&m.LastHitAt, &m.CreatedAt, &m.PromotedAt, &m.PromotedToID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PruneMistakes — soft-delete row tier='raw' yang last_hit_at lebih lama dari
// olderThan (e.g. 90 days). Tier 'reviewed' / 'promoted' tidak di-prune (sakral).
// Return count deleted.
func (s *Store) PruneMistakes(olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE mistakes_local SET deleted_at = CURRENT_TIMESTAMP, deleted_by = 'prune-cron'
		 WHERE deleted_at IS NULL AND tier = 'raw' AND last_hit_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("prune mistakes: %w", err)
	}
	return res.RowsAffected()
}

// CountMistakes — count non-deleted, optional filter tier. Buat metric.
func (s *Store) CountMistakes(tier string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT COUNT(*) FROM mistakes_local WHERE deleted_at IS NULL`
	args := []any{}
	if tier != "" {
		query += ` AND tier = ?`
		args = append(args, tier)
	}

	var n int64
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
