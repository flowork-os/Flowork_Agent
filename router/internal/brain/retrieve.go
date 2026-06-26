// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"context"
	"database/sql"
	"fmt"
)

type Snippet struct {
	DrawerID string  `json:"drawer_id"`
	Wing     string  `json:"wing"`
	Room     string  `json:"room"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
}

type RetrieveOpts struct {
	Limit int

	Wings []string

	MaxContentLen int
}

func Retrieve(ctx context.Context, db *sql.DB, query string, opts RetrieveOpts) ([]Snippet, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 6
	}
	tokens := ftsTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	snips, err := runFTS(ctx, db, joinFTS(tokens, "AND"), opts.Wings, limit, opts.MaxContentLen)
	if err != nil {
		return nil, err
	}
	if len(snips) == 0 && len(tokens) > 1 {
		snips, err = runFTS(ctx, db, joinFTS(tokens, "OR"), opts.Wings, limit, opts.MaxContentLen)
		if err != nil {
			return nil, err
		}
	}
	return snips, nil
}

func runFTS(ctx context.Context, db *sql.DB, match string, wings []string, limit, maxLen int) ([]Snippet, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if len(wings) > 0 {
		placeholders := ""
		args := []any{match}
		for i, w := range wings {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, w)
		}
		args = append(args, limit)
		stmt := fmt.Sprintf(`SELECT %[1]s.drawer_id, %[1]s.wing, %[1]s.room, %[1]s.content, bm25(%[1]s) AS score
			FROM %[1]s JOIN drawers d ON d.id = %[1]s.drawer_id
			WHERE %[1]s MATCH ? AND d.deleted_at IS NULL AND %[1]s.wing IN (%[2]s)
			ORDER BY score LIMIT ?`, ftsTable, placeholders)
		rows, err = db.QueryContext(ctx, stmt, args...)
	} else {
		stmt := fmt.Sprintf(`SELECT %[1]s.drawer_id, %[1]s.wing, %[1]s.room, %[1]s.content, bm25(%[1]s) AS score
			FROM %[1]s JOIN drawers d ON d.id = %[1]s.drawer_id
			WHERE %[1]s MATCH ? AND d.deleted_at IS NULL
			ORDER BY score LIMIT ?`, ftsTable)
		rows, err = db.QueryContext(ctx, stmt, match, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("brain retrieve: %w", err)
	}
	defer rows.Close()

	var out []Snippet
	for rows.Next() {
		var s Snippet
		var bm25 float64
		if err := rows.Scan(&s.DrawerID, &s.Wing, &s.Room, &s.Content, &bm25); err != nil {
			continue
		}
		if bm25 < 0 {
			bm25 = -bm25
		}
		s.Score = 1.0 / (1.0 + bm25)
		if maxLen > 0 {
			s.Content = truncateRunes(s.Content, maxLen)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("brain retrieve rows: %w", err)
	}
	return out, nil
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
