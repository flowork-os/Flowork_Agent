// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-03
// Reason: Roadmap 2 B4 skill grow-from-patterns. Verified: successful tool freq
//   -> candidates (failed/rare excluded). Pair w/ Fase 8 curator. Extend -> file baru.
//
// tool_patterns.go — Roadmap 2 Fase B4: skill GROW dari pola tool sukses.
//
// Curator (skills_curate.go, Fase 8) udah handle GRADE/CONSOLIDATE/ARCHIVE.
// Yang kurang buat B4: skill TUMBUH dari pola sukses. File ini mining
// tool_invocations (error_text='' = sukses) → tool yang sering dipake sukses
// jadi KANDIDAT skill (di-suggest, BUKAN auto-create — auto-create = YAGNI per
// keputusan Fase 8). Derive on-the-fly dari tool_invocations (no tabel baru).

package agentdb

import "fmt"

// SkillCandidate — usulan skill dari pola tool sukses berulang.
type SkillCandidate struct {
	ToolName     string `json:"tool_name"`
	SuccessCount int    `json:"success_count"`
	LastUsed     string `json:"last_used"`
	Suggestion   string `json:"suggestion"`
}

// SuggestSkillCandidates — tool yang dipake SUKSES >= minCount kali jadi
// kandidat skill. Urut paling sering. Rule-based, on-demand (anti over-prompt).
func (s *Store) SuggestSkillCandidates(minCount, limit int) ([]SkillCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if minCount < 2 {
		minCount = 2
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT tool_name, COUNT(*) c, MAX(invoked_at) last_used
		   FROM tool_invocations
		  WHERE deleted_at IS NULL AND error_text = ''
		  GROUP BY tool_name
		 HAVING c >= ?
		  ORDER BY c DESC, last_used DESC
		  LIMIT ?`, minCount, limit)
	if err != nil {
		return nil, fmt.Errorf("suggest skill candidates: %w", err)
	}
	defer rows.Close()

	out := []SkillCandidate{}
	for rows.Next() {
		var c SkillCandidate
		if err := rows.Scan(&c.ToolName, &c.SuccessCount, &c.LastUsed); err != nil {
			return nil, err
		}
		c.Suggestion = fmt.Sprintf(
			"Lo udah pakai '%s' sukses %d kali — pertimbangin bikin skill (alur/template) di sekitar tool ini biar konsisten.",
			c.ToolName, c.SuccessCount)
		out = append(out, c)
	}
	return out, rows.Err()
}
