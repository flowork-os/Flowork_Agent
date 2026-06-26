// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import "fmt"

type SkillCandidate struct {
	ToolName     string `json:"tool_name"`
	SuccessCount int    `json:"success_count"`
	LastUsed     string `json:"last_used"`
	Suggestion   string `json:"suggestion"`
}

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
