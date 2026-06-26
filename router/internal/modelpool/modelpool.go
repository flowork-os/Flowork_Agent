// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package modelpool

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

const AlgoVersion = "v1"

type Model struct {
	ID             int64   `json:"id"`
	ModelID        string  `json:"model_id"`
	ModelName      string  `json:"model_name"`
	Category       string  `json:"category"`
	ContextWindow  int64   `json:"context_window"`
	CostPrompt     float64 `json:"cost_prompt"`
	CostCompletion float64 `json:"cost_completion"`
	IsFree         bool    `json:"is_free"`
	CreatedAt      string  `json:"created_at"`
}

type UpsertOpts struct {
	ModelID        string
	ModelName      string
	Category       string
	ContextWindow  int64
	CostPrompt     float64
	CostCompletion float64
	IsFree         bool
}

func Upsert(ctx context.Context, opts UpsertOpts) (int64, bool, error) {
	modelID := strings.TrimSpace(opts.ModelID)
	modelName := strings.TrimSpace(opts.ModelName)
	if modelID == "" || modelName == "" {
		return 0, false, fmt.Errorf("model_id + model_name required")
	}
	const maxLen = 256
	if len(modelID) > maxLen {
		modelID = modelID[:maxLen]
	}
	if len(modelName) > maxLen {
		modelName = modelName[:maxLen]
	}
	category := strings.TrimSpace(opts.Category)
	if category == "" {
		category = "Text/Chat"
	}

	db, err := brain.OpenRW()
	if err != nil {
		return 0, false, err
	}

	var existingID int64
	qerr := db.QueryRowContext(ctx,
		`SELECT id FROM model_pool WHERE model_id = ?`, modelID,
	).Scan(&existingID)

	isFreeInt := 0
	if opts.IsFree {
		isFreeInt = 1
	}

	if qerr == sql.ErrNoRows {
		res, ierr := db.ExecContext(ctx,
			`INSERT INTO model_pool(model_id, model_name, category, context_window,
			                        cost_prompt, cost_completion, is_free)
			 VALUES(?, ?, ?, ?, ?, ?, ?)`,
			modelID, modelName, category, opts.ContextWindow,
			opts.CostPrompt, opts.CostCompletion, isFreeInt,
		)
		if ierr != nil {
			return 0, false, fmt.Errorf("insert model: %w", ierr)
		}
		newID, _ := res.LastInsertId()
		return newID, true, nil
	}
	if qerr != nil {
		return 0, false, fmt.Errorf("lookup model: %w", qerr)
	}

	_, uerr := db.ExecContext(ctx,
		`UPDATE model_pool SET
		    model_name      = ?,
		    category        = ?,
		    context_window  = ?,
		    cost_prompt     = ?,
		    cost_completion = ?,
		    is_free         = ?
		 WHERE id = ?`,
		modelName, category, opts.ContextWindow,
		opts.CostPrompt, opts.CostCompletion, isFreeInt, existingID,
	)
	if uerr != nil {
		return 0, false, fmt.Errorf("update model: %w", uerr)
	}
	return existingID, false, nil
}

func Get(ctx context.Context, modelID string) (Model, error) {
	if modelID == "" {
		return Model{}, fmt.Errorf("model_id required")
	}
	db, err := brain.OpenRW()
	if err != nil {
		return Model{}, err
	}
	var m Model
	var isFreeInt int
	rerr := db.QueryRowContext(ctx,
		`SELECT id, model_id, model_name, category, context_window,
		        cost_prompt, cost_completion, is_free, created_at
		 FROM model_pool WHERE model_id = ?`,
		modelID,
	).Scan(&m.ID, &m.ModelID, &m.ModelName, &m.Category, &m.ContextWindow,
		&m.CostPrompt, &m.CostCompletion, &isFreeInt, &m.CreatedAt)
	if rerr == sql.ErrNoRows {
		return Model{ModelID: modelID}, nil
	}
	if rerr != nil {
		return Model{}, fmt.Errorf("get model: %w", rerr)
	}
	m.IsFree = isFreeInt != 0
	return m, nil
}

type ListOpts struct {
	Category   string
	IsFreeOnly bool
	MaxCost    float64
	Limit      int
}

func List(ctx context.Context, opts ListOpts) ([]Model, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	db, err := brain.OpenRW()
	if err != nil {
		return nil, err
	}

	query := `SELECT id, model_id, model_name, category, context_window,
	                 cost_prompt, cost_completion, is_free, created_at
	          FROM model_pool WHERE 1=1`
	args := []any{}
	if opts.Category != "" {
		query += ` AND category = ?`
		args = append(args, opts.Category)
	}
	if opts.IsFreeOnly {
		query += ` AND is_free = 1`
	}
	if opts.MaxCost > 0 {
		query += ` AND cost_prompt <= ? AND cost_completion <= ?`
		args = append(args, opts.MaxCost, opts.MaxCost)
	}
	query += ` ORDER BY cost_prompt ASC, model_id ASC LIMIT ?`
	args = append(args, limit)

	rows, qerr := db.QueryContext(ctx, query, args...)
	if qerr != nil {
		return nil, fmt.Errorf("query model_pool: %w", qerr)
	}
	defer rows.Close()

	var out []Model
	for rows.Next() {
		var m Model
		var isFreeInt int
		if err := rows.Scan(&m.ID, &m.ModelID, &m.ModelName, &m.Category, &m.ContextWindow,
			&m.CostPrompt, &m.CostCompletion, &isFreeInt, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.IsFree = isFreeInt != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func Delete(ctx context.Context, modelID string) (int64, error) {
	if modelID == "" {
		return 0, fmt.Errorf("model_id required")
	}
	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	res, derr := db.ExecContext(ctx,
		`DELETE FROM model_pool WHERE model_id = ?`, modelID,
	)
	if derr != nil {
		return 0, fmt.Errorf("delete model: %w", derr)
	}
	return res.RowsAffected()
}

func Count(ctx context.Context, category string) (int64, error) {
	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	query := `SELECT COUNT(*) FROM model_pool`
	args := []any{}
	if category != "" {
		query += ` WHERE category = ?`
		args = append(args, category)
	}
	var n int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

var _ = time.Now
