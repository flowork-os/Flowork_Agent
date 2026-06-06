// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-29
// Reason: Section 7 Agent extension (mistakes promote helpers).
//   API stable: SetMistakePromoted (idempotent UPDATE), Eligible list
//   (filter tier=raw + hit_count threshold + not yet promoted, cap 200).
//   File baru — ngga modify mistakes.go LOCKED.
//
// mistakes_promote.go — Section 7 roadmap extension: promote mistakes
// lokal ke router brain antibody.
//
// PURPOSE:
//   Mistakes.go LOCKED — extend via new file di same package.
//   `SetMistakePromoted(id, routerID)`: update tier='promoted' + promoted_at
//   + promoted_to_id setelah successful POST ke Router /api/mistakes/submit.
//   `ListMistakesEligibleForPromote(minHitCount)`: list tier='raw' yang
//   hit_count meets threshold, ready untuk push.
//
// SEMANTIC:
//   - Sekali promoted, tier='promoted' (locked). Mistake-local stays tapi
//     ngga di-include di future promote sweep.
//   - Caller (kernel cron) cek `promoted_to_id == ''` selain tier untuk
//     defense in depth (kalau tier ke-reset manual lewat raw SQL).
//
// Source: Flowork_Agent/roadmap.md Section 7 phase 1.

package agentdb

import (
	"fmt"
	"time"
)

// SetMistakePromoted — mark mistake lokal sebagai promoted. Pakai setelah
// successful submit ke Router (caller dapat router-side row id).
//
// Idempotent: kalau tier sudah 'promoted', return nil tanpa modify.
func (s *Store) SetMistakePromoted(id int64, promotedToID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id <= 0 {
		return fmt.Errorf("mistake id required")
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	promotedStr := fmt.Sprintf("%d", promotedToID)

	_, err := s.db.Exec(
		`UPDATE mistakes_local SET
		    tier           = 'promoted',
		    promoted_at    = ?,
		    promoted_to_id = ?
		 WHERE id = ? AND tier != 'promoted' AND deleted_at IS NULL`,
		ts, promotedStr, id,
	)
	if err != nil {
		return fmt.Errorf("set mistake promoted: %w", err)
	}
	return nil
}

// ListMistakesEligibleForPromote — list tier='raw' dengan hit_count ≥
// minHitCount, deleted_at IS NULL, promoted_to_id kosong. Order: hit_count
// DESC (most-frequent first), then last_hit_at DESC. Cap default 50.
func (s *Store) ListMistakesEligibleForPromote(minHitCount int64, limit int) ([]Mistake, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if minHitCount < 1 {
		minHitCount = 3
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.Query(
		`SELECT id, category, title, content, context_origin, tier, hit_count,
		        last_hit_at, created_at,
		        COALESCE(promoted_at, ''), COALESCE(promoted_to_id, '')
		 FROM mistakes_local
		 WHERE deleted_at IS NULL
		   AND tier = 'raw'
		   AND hit_count >= ?
		   AND (promoted_to_id IS NULL OR promoted_to_id = '')
		 ORDER BY hit_count DESC, last_hit_at DESC
		 LIMIT ?`,
		minHitCount, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query eligible: %w", err)
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
