// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import "context"

func ListByWing(ctx context.Context, wing, roomLike string, limit, offset, maxContentLen int) ([]Snippet, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	db, err := Open()
	if err != nil {
		return nil, err
	}
	query := `SELECT id, wing, room, content FROM drawers WHERE deleted_at IS NULL AND wing = ? `
	args := []any{wing}
	if roomLike != "" {
		query += `AND room LIKE ? `
		args = append(args, roomLike)
	}
	query += `ORDER BY importance DESC, filed_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := db.QueryContext(ctx, query, args...)
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
