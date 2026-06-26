// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"context"
	"fmt"
	"time"
)

var mistakeCategoryWhitelist = map[string]struct{}{
	"logic":       {},
	"safety":      {},
	"performance": {},
	"security":    {},
	"ux":          {},
	"governance":  {},
}

func IsValidMistakeCategory(category string) bool {
	_, ok := mistakeCategoryWhitelist[category]
	return ok
}

func ListMistakeCategories() []string {
	out := make([]string, 0, len(mistakeCategoryWhitelist))
	for k := range mistakeCategoryWhitelist {
		out = append(out, k)
	}

	return out
}

type Mistake struct {
	ID                   int64  `json:"id"`
	Category             string `json:"category"`
	Title                string `json:"title"`
	Content              string `json:"content"`
	SourceAgentID        string `json:"source_agent_id"`
	HitCount             int64  `json:"hit_count"`
	Tier                 string `json:"tier"`
	ReviewedAt           string `json:"reviewed_at,omitempty"`
	PromotedToAntibodyID string `json:"promoted_to_antibody_id,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

func SubmitMistake(ctx context.Context, category, title, content, sourceAgentID string, hitCount int64) (int64, bool, error) {
	if category == "" || title == "" || content == "" || sourceAgentID == "" {
		return 0, false, fmt.Errorf("category + title + content + source_agent_id required")
	}

	const maxAgentIDBytes = 128
	if len(sourceAgentID) > maxAgentIDBytes {
		return 0, false, fmt.Errorf("source_agent_id must be <= %d bytes", maxAgentIDBytes)
	}
	if !IsValidMistakeCategory(category) {
		return 0, false, fmt.Errorf("category %q not in whitelist", category)
	}
	const (
		minHitCount = 3
		maxHitCount = 1_000_000
	)
	if hitCount < minHitCount {
		return 0, false, fmt.Errorf("hit_count must be >= %d (got %d)", minHitCount, hitCount)
	}
	if hitCount > maxHitCount {
		return 0, false, fmt.Errorf("hit_count must be <= %d (got %d)", maxHitCount, hitCount)
	}

	const (
		maxContentBytes = 8 * 1024
		maxTitleBytes   = 256
	)
	if len(content) > maxContentBytes {
		content = content[:maxContentBytes] + "…[truncated]"
	}
	if len(title) > maxTitleBytes {
		title = title[:maxTitleBytes] + "…"
	}

	db, err := OpenRW()
	if err != nil {
		return 0, false, err
	}
	ts := time.Now().UTC().Format(time.RFC3339)

	var id int64
	var newHitCount int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO mistakes_journal(category, title, content, source_agent_id, hit_count, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(category, title) DO UPDATE SET
		     content    = excluded.content,
		     hit_count  = mistakes_journal.hit_count + excluded.hit_count,
		     updated_at = excluded.updated_at,
		     deleted_at = NULL
		 RETURNING id, hit_count`,
		category, title, content, sourceAgentID, hitCount, ts, ts,
	).Scan(&id, &newHitCount); err != nil {
		return 0, false, fmt.Errorf("upsert mistake: %w", err)
	}

	isNew := newHitCount == hitCount
	return id, isNew, nil
}

func ListMistakes(ctx context.Context, tier, sourceAgentID string, limit int) ([]Mistake, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	db, err := OpenRW()
	if err != nil {
		return nil, err
	}

	query := `SELECT id, category, title, content, source_agent_id, hit_count, tier,
	                 COALESCE(reviewed_at, ''), COALESCE(promoted_to_antibody_id, ''),
	                 created_at, updated_at
	          FROM mistakes_journal WHERE deleted_at IS NULL`
	args := []any{}
	if tier != "" {
		query += ` AND tier = ?`
		args = append(args, tier)
	}
	if sourceAgentID != "" {
		query += ` AND source_agent_id = ?`
		args = append(args, sourceAgentID)
	}
	query += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)

	rows, qerr := db.QueryContext(ctx, query, args...)
	if qerr != nil {
		return nil, fmt.Errorf("query mistakes: %w", qerr)
	}
	defer rows.Close()

	var out []Mistake
	for rows.Next() {
		var m Mistake
		if err := rows.Scan(&m.ID, &m.Category, &m.Title, &m.Content,
			&m.SourceAgentID, &m.HitCount, &m.Tier,
			&m.ReviewedAt, &m.PromotedToAntibodyID,
			&m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func CountMistakes(ctx context.Context, tier string) (int64, error) {
	db, err := OpenRW()
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(*) FROM mistakes_journal WHERE deleted_at IS NULL`
	args := []any{}
	if tier != "" {
		query += ` AND tier = ?`
		args = append(args, tier)
	}
	var n int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
