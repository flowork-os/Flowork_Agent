// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Brain drawer/embeddings/skills.

// Brain read-views (typed memory + personas).

package brain

import (
	"context"
)

// ListByType returns Memory-Palace drawers of a given mem_type (typed memory:
// user / feedback / project / reference / knowledge / doctrine …). Read-only.
// limit<=0 → 50. Mirrors flowork's "Typed Memory" tab.
func ListByType(ctx context.Context, memType string, limit, maxContentLen int) ([]Snippet, error) {
	if limit <= 0 {
		limit = 50
	}
	db, err := Open()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, wing, room, content FROM drawers
		WHERE deleted_at IS NULL AND mem_type = ?
		ORDER BY importance DESC, filed_at DESC LIMIT ?`, memType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snippet
	for rows.Next() {
		var s Snippet
		if err := rows.Scan(&s.DrawerID, &s.Wing, &s.Room, &s.Content); err != nil {
			continue
		}
		if maxContentLen > 0 {
			s.Content = truncateRunes(s.Content, maxContentLen)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Persona — a canonical prompt template (flowork "Prompt Library" / Identity).
type Persona struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	UpdatedAt string `json:"updatedAt"`
}

// ListPersonas returns the canonical prompt templates (subagent personas).
// Read-only. Returns nil (not an error) if the table is absent.
func ListPersonas(ctx context.Context) ([]Persona, error) {
	db, err := Open()
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT name, content, COALESCE(source_path,''), COALESCE(updated_at,'')
		FROM prompt_templates ORDER BY name ASC`)
	if err != nil {
		return nil, nil // table absent on this DB → no personas, not fatal
	}
	defer rows.Close()
	var out []Persona
	for rows.Next() {
		var p Persona
		if err := rows.Scan(&p.Name, &p.Content, &p.Source, &p.UpdatedAt); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
