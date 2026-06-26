// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"context"
	"fmt"
)

type RescoreOpts struct {
	Wing string

	Limit int

	ForceOverride bool
}

const preserveThreshold = 1.5

type RescoreReport struct {
	Scanned     int            `json:"scanned"`
	Updated     int            `json:"updated"`
	Unchanged   int            `json:"unchanged"`
	Preserved   int            `json:"preserved"`
	Errors      []string       `json:"errors,omitempty"`
	SampleDelta []RescoreDelta `json:"sample_delta,omitempty"`
}

type RescoreDelta struct {
	DrawerID string  `json:"drawer_id"`
	Before   float64 `json:"before"`
	After    float64 `json:"after"`
}

type scorerFn func(content, sourceType string) float64

func RescoreBatch(ctx context.Context, opts RescoreOpts, scorer scorerFn) (RescoreReport, error) {
	if scorer == nil {
		return RescoreReport{}, fmt.Errorf("scorer function required")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}

	db, err := OpenRW()
	if err != nil {
		return RescoreReport{}, err
	}

	query := `SELECT id, content, source_type, importance
	          FROM drawers
	          WHERE deleted_at IS NULL AND quarantined = 0`
	args := []any{}
	if opts.Wing != "" {
		query += ` AND wing = ?`
		args = append(args, opts.Wing)
	}
	query += ` ORDER BY filed_at DESC LIMIT ?`
	args = append(args, limit)

	rows, qerr := db.QueryContext(ctx, query, args...)
	if qerr != nil {
		return RescoreReport{}, fmt.Errorf("query drawers: %w", qerr)
	}

	type drawerRow struct {
		id         string
		content    string
		sourceType string
		importance float64
	}
	var drawers []drawerRow
	for rows.Next() {
		var d drawerRow
		if err := rows.Scan(&d.id, &d.content, &d.sourceType, &d.importance); err != nil {
			rows.Close()
			return RescoreReport{}, fmt.Errorf("scan drawer: %w", err)
		}
		drawers = append(drawers, d)
	}
	rows.Close()
	if rerr := rows.Err(); rerr != nil {
		return RescoreReport{}, fmt.Errorf("iterate drawers: %w", rerr)
	}

	rep := RescoreReport{Scanned: len(drawers)}

	type pendingUpdate struct {
		id       string
		newScore float64
		oldScore float64
	}
	var pending []pendingUpdate
	for _, d := range drawers {
		newScore := scorer(d.content, d.sourceType)
		delta := absDelta(newScore, d.importance)
		if delta < 0.01 {
			rep.Unchanged++
			continue
		}
		if !opts.ForceOverride && delta > preserveThreshold {

			rep.Preserved++
			continue
		}
		pending = append(pending, pendingUpdate{
			id:       d.id,
			newScore: newScore,
			oldScore: d.importance,
		})
	}

	if len(pending) == 0 {
		return rep, nil
	}

	tx, terr := db.BeginTx(ctx, nil)
	if terr != nil {
		return rep, fmt.Errorf("begin tx: %w", terr)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, perr := tx.PrepareContext(ctx, `UPDATE drawers SET importance = ? WHERE id = ?`)
	if perr != nil {
		return rep, fmt.Errorf("prepare update: %w", perr)
	}
	defer stmt.Close()

	for _, p := range pending {
		if _, uerr := stmt.ExecContext(ctx, p.newScore, p.id); uerr != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("drawer %s: %s", p.id, uerr.Error()))
			continue
		}
		rep.Updated++
		if len(rep.SampleDelta) < 20 {
			rep.SampleDelta = append(rep.SampleDelta, RescoreDelta{
				DrawerID: p.id,
				Before:   p.oldScore,
				After:    p.newScore,
			})
		}
	}

	if cerr := tx.Commit(); cerr != nil {
		return rep, fmt.Errorf("commit tx: %w", cerr)
	}
	tx = nil
	return rep, nil
}

func absDelta(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
