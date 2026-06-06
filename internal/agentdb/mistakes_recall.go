// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B2 mistakes recall. Verified: add 2x→hit_count, SearchMistakes
//   recall by context + hit_count order. Extend → file baru, JANGAN modify ini.
//
// mistakes_recall.go — Roadmap 2 Fase B2: recall mistakes pas konteks mirip.
//
// mistakes.go (LOCKED) udah handle Add (dedup + hit_count) / List / Prune.
// Yang kurang buat B2: RECALL berdasarkan konteks — "dulu lo salah X, solusinya
// Y" pas situasi mirip muncul lagi. File ini nambah SearchMistakes (keyword
// LIKE, urut hit_count) tanpa nyentuh file locked.
//
// Anti over-prompt: recall = on-demand (tool mistake_recall), BUKAN auto-inject.

package agentdb

import (
	"fmt"
	"strings"
)

// SearchMistakes cari mistake live yang match keyword di konteks/query.
// Match = token (≥3 char) muncul di title ATAU content (LIKE, case-insensitive).
// Urut: hit_count DESC (yang sering keulang = paling penting di-warn) lalu recent.
func (s *Store) SearchMistakes(query string, limit int) ([]Mistake, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	// Tokenize: ambil kata ≥3 char, lowercase.
	var toks []string
	for _, f := range strings.Fields(strings.ToLower(query)) {
		f = strings.Trim(f, ".,:;!?()[]{}\"'`")
		if len(f) >= 3 {
			toks = append(toks, f)
		}
	}
	if len(toks) == 0 {
		return []Mistake{}, nil
	}

	// WHERE (deleted live) AND (tok1 match OR tok2 match OR …).
	var conds []string
	var args []any
	for _, t := range toks {
		conds = append(conds, "(lower(title) LIKE ? OR lower(content) LIKE ?)")
		like := "%" + t + "%"
		args = append(args, like, like)
	}
	q := `SELECT id, category, title, content, context_origin, tier, hit_count,
	             last_hit_at, created_at
	        FROM mistakes_local
	       WHERE deleted_at IS NULL AND (` + strings.Join(conds, " OR ") + `)
	       ORDER BY hit_count DESC, last_hit_at DESC
	       LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search mistakes: %w", err)
	}
	defer rows.Close()

	out := []Mistake{}
	for rows.Next() {
		var m Mistake
		if err := rows.Scan(&m.ID, &m.Category, &m.Title, &m.Content, &m.ContextOrigin,
			&m.Tier, &m.HitCount, &m.LastHitAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
