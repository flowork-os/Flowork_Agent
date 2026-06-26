// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package constitution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

const AmendAlgoVersion = "amend-v1"

var ErrInvalidInput = errors.New("invalid input")

const (
	KindReword    = "reword"
	KindAmplitude = "amplitude"
	KindDelete    = "delete"
)

const (
	maxAmendContent = 16 * 1024
	maxAmendField   = 512
	maxAmplitude    = 1e7
)

type Amendment struct {
	ID            int64   `json:"id"`
	TargetID      int64   `json:"target_id"`
	TargetSection string  `json:"target_section"`
	Kind          string  `json:"kind"`
	OldContent    string  `json:"old_content,omitempty"`
	OldAmplitude  float64 `json:"old_amplitude"`
	NewContent    string  `json:"new_content,omitempty"`
	NewAmplitude  float64 `json:"new_amplitude,omitempty"`
	Rationale     string  `json:"rationale,omitempty"`
	Signer        string  `json:"signer,omitempty"`
	Status        string  `json:"status"`
	Applied       int     `json:"applied"`
	CreatedAt     string  `json:"created_at,omitempty"`
	DecidedAt     string  `json:"decided_at,omitempty"`
	DecidedBy     string  `json:"decided_by,omitempty"`
}

type ProposeAmendOpts struct {
	TargetID     int64
	Kind         string
	NewContent   string
	NewAmplitude float64
	Rationale    string
	Signer       string
}

func ensureAmendSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS constitution_amendment (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		target_id      INTEGER NOT NULL,
		target_section TEXT,
		kind           TEXT NOT NULL,
		old_content    TEXT,
		old_amplitude  REAL,
		new_content    TEXT,
		new_amplitude  REAL,
		rationale      TEXT,
		signer         TEXT,
		status         TEXT NOT NULL DEFAULT 'pending',
		created_at     TEXT,
		decided_at     TEXT,
		decided_by     TEXT,
		applied        INTEGER DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("ensure amend schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_amend_status ON constitution_amendment(status)`); err != nil {
		return fmt.Errorf("ensure amend idx_status: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_amend_target ON constitution_amendment(target_id)`); err != nil {
		return fmt.Errorf("ensure amend idx_target: %w", err)
	}
	return nil
}

type targetRow struct {
	section   string
	content   string
	amplitude float64
	deleted   bool
	pending   int
}

func lookupTargetQ(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (targetRow, error) {
	var tr targetRow
	var deletedAt sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT section, content, amplitude, deleted_at, pending_quorum_review
		 FROM constitution WHERE id = ?`, id,
	).Scan(&tr.section, &tr.content, &tr.amplitude, &deletedAt, &tr.pending)
	if err == sql.ErrNoRows {
		return tr, fmt.Errorf("%w: target rule id %d not found", ErrInvalidInput, id)
	}
	if err != nil {
		return tr, fmt.Errorf("lookup target: %w", err)
	}
	tr.deleted = deletedAt.Valid
	return tr, nil
}

func ProposeAmendment(ctx context.Context, opts ProposeAmendOpts) (int64, error) {
	if opts.TargetID <= 0 {
		return 0, fmt.Errorf("%w: target_id required (positive int)", ErrInvalidInput)
	}
	kind := strings.TrimSpace(opts.Kind)
	switch kind {
	case KindReword, KindAmplitude, KindDelete:
	default:
		return 0, fmt.Errorf("%w: kind must be 'reword', 'amplitude', or 'delete'", ErrInvalidInput)
	}

	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	if err := ensureAmendSchema(ctx, db); err != nil {
		return 0, err
	}

	tr, err := lookupTargetQ(ctx, db, opts.TargetID)
	if err != nil {
		return 0, err
	}
	if tr.deleted {
		return 0, fmt.Errorf("%w: target rule id %d already soft-deleted", ErrInvalidInput, opts.TargetID)
	}
	if tr.pending == 1 {
		return 0, fmt.Errorf("%w: target rule id %d is itself a pending proposal; settle it via /vote first", ErrInvalidInput, opts.TargetID)
	}

	newContent := strings.TrimSpace(opts.NewContent)
	newAmp := opts.NewAmplitude
	switch kind {
	case KindReword:
		if newContent == "" {
			return 0, fmt.Errorf("%w: new_content required for reword", ErrInvalidInput)
		}
		if len(newContent) > maxAmendContent {
			return 0, fmt.Errorf("%w: new_content too large (max %d bytes)", ErrInvalidInput, maxAmendContent)
		}
		if newContent == strings.TrimSpace(tr.content) {
			return 0, fmt.Errorf("%w: new_content identical to current content; nothing to reword", ErrInvalidInput)
		}
		newAmp = 0
	case KindAmplitude:
		if newAmp <= 0 {
			return 0, fmt.Errorf("%w: new_amplitude required (> 0) for amplitude change", ErrInvalidInput)
		}
		if newAmp > maxAmplitude {
			return 0, fmt.Errorf("%w: new_amplitude too large (max %g)", ErrInvalidInput, maxAmplitude)
		}
		if newAmp == tr.amplitude {
			return 0, fmt.Errorf("%w: new_amplitude identical to current (%g); nothing to change", ErrInvalidInput, tr.amplitude)
		}
		newContent = ""
	case KindDelete:
		newContent, newAmp = "", 0
	}

	var dup int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM constitution_amendment WHERE target_id = ? AND status = 'pending'`,
		opts.TargetID,
	).Scan(&dup); err != nil {
		return 0, fmt.Errorf("dup check: %w", err)
	}
	if dup > 0 {
		return 0, fmt.Errorf("%w: an amendment is already pending for rule id %d; settle it first", ErrInvalidInput, opts.TargetID)
	}

	rationale := strings.TrimSpace(opts.Rationale)
	if len(rationale) > maxAmendContent {
		return 0, fmt.Errorf("%w: rationale too large (max %d bytes)", ErrInvalidInput, maxAmendContent)
	}
	signer := strings.TrimSpace(opts.Signer)
	if signer == "" {
		signer = "anonymous"
	}
	if len(signer) > maxAmendField {
		signer = signer[:maxAmendField]
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, ierr := db.ExecContext(ctx,
		`INSERT INTO constitution_amendment
		 (target_id, target_section, kind, old_content, old_amplitude,
		  new_content, new_amplitude, rationale, signer, status, created_at, applied)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, 0)`,
		opts.TargetID, tr.section, kind, tr.content, tr.amplitude,
		newContent, newAmp, rationale, signer, now,
	)
	if ierr != nil {
		return 0, fmt.Errorf("insert amendment: %w", ierr)
	}
	return res.LastInsertId()
}

var validListStatus = map[string]bool{"": true, "pending": true, "approved": true, "rejected": true}

func ListAmendments(ctx context.Context, status string, limit int) ([]Amendment, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	status = strings.TrimSpace(status)
	if !validListStatus[status] {
		return nil, fmt.Errorf("%w: status must be pending|approved|rejected (or empty for all)", ErrInvalidInput)
	}
	db, err := brain.OpenRW()
	if err != nil {
		return nil, err
	}
	if err := ensureAmendSchema(ctx, db); err != nil {
		return nil, err
	}

	q := `SELECT id, target_id, COALESCE(target_section,''), kind,
	             COALESCE(old_content,''), COALESCE(old_amplitude,0),
	             COALESCE(new_content,''), COALESCE(new_amplitude,0),
	             COALESCE(rationale,''), COALESCE(signer,''), status, applied,
	             COALESCE(created_at,''), COALESCE(decided_at,''), COALESCE(decided_by,'')
	      FROM constitution_amendment`
	var args []any
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id ASC LIMIT ?`
	args = append(args, limit)

	rows, qerr := db.QueryContext(ctx, q, args...)
	if qerr != nil {
		return nil, fmt.Errorf("query amendments: %w", qerr)
	}
	defer rows.Close()

	var out []Amendment
	for rows.Next() {
		var a Amendment
		if err := rows.Scan(&a.ID, &a.TargetID, &a.TargetSection, &a.Kind,
			&a.OldContent, &a.OldAmplitude, &a.NewContent, &a.NewAmplitude,
			&a.Rationale, &a.Signer, &a.Status, &a.Applied,
			&a.CreatedAt, &a.DecidedAt, &a.DecidedBy); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type AmendVoteOpts struct {
	AmendmentID int64
	Action      string
	VoterID     string
}

type AmendVoteResult struct {
	AmendmentID int64  `json:"amendment_id"`
	TargetID    int64  `json:"target_id"`
	Kind        string `json:"kind"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	Applied     bool   `json:"applied"`
}

func VoteAmendment(ctx context.Context, opts AmendVoteOpts) (AmendVoteResult, error) {
	r := AmendVoteResult{AmendmentID: opts.AmendmentID, Action: opts.Action}
	if opts.AmendmentID <= 0 {
		return r, fmt.Errorf("%w: amendment_id required", ErrInvalidInput)
	}
	action := strings.TrimSpace(opts.Action)
	if action != "approve" && action != "reject" {
		return r, fmt.Errorf("%w: action must be 'approve' or 'reject'", ErrInvalidInput)
	}
	voter := strings.TrimSpace(opts.VoterID)
	if voter == "" {
		voter = "anonymous"
	}
	if len(voter) > maxAmendField {
		voter = voter[:maxAmendField]
	}

	db, err := brain.OpenRW()
	if err != nil {
		return r, err
	}
	if err := ensureAmendSchema(ctx, db); err != nil {
		return r, err
	}

	var (
		targetID   int64
		kind       string
		newContent sql.NullString
		newAmp     sql.NullFloat64
		status     string
	)
	qerr := db.QueryRowContext(ctx,
		`SELECT target_id, kind, new_content, new_amplitude, status
		 FROM constitution_amendment WHERE id = ?`, opts.AmendmentID,
	).Scan(&targetID, &kind, &newContent, &newAmp, &status)
	if qerr == sql.ErrNoRows {
		return r, fmt.Errorf("%w: amendment not found", ErrInvalidInput)
	}
	if qerr != nil {
		return r, fmt.Errorf("lookup amendment: %w", qerr)
	}
	r.TargetID = targetID
	r.Kind = kind

	if status != "pending" {
		r.Status = "no-op"
		return r, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if action == "reject" {
		if _, uerr := db.ExecContext(ctx,
			`UPDATE constitution_amendment SET status='rejected', decided_at=?, decided_by=?
			 WHERE id=? AND status='pending'`, now, voter, opts.AmendmentID,
		); uerr != nil {
			return r, fmt.Errorf("reject: %w", uerr)
		}
		r.Status = "rejected"
		return r, nil
	}

	tx, terr := db.BeginTx(ctx, nil)
	if terr != nil {
		return r, fmt.Errorf("begin tx: %w", terr)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	tr, lerr := lookupTargetQ(ctx, tx, targetID)
	if lerr != nil {
		return r, lerr
	}
	if tr.deleted {
		return r, fmt.Errorf("%w: target rule id %d already deleted; cannot apply", ErrInvalidInput, targetID)
	}
	if tr.pending == 1 {
		return r, fmt.Errorf("%w: target rule id %d is now a pending proposal; cannot amend", ErrInvalidInput, targetID)
	}

	var applyRes sql.Result
	var aerr error
	switch kind {
	case KindReword:
		if !newContent.Valid || strings.TrimSpace(newContent.String) == "" {
			return r, fmt.Errorf("%w: reword amendment missing new_content", ErrInvalidInput)
		}
		applyRes, aerr = tx.ExecContext(ctx,
			`UPDATE constitution SET content=?, amplitude=? WHERE id=? AND deleted_at IS NULL`,
			newContent.String, tr.amplitude, targetID)
	case KindAmplitude:
		if !newAmp.Valid || newAmp.Float64 <= 0 {
			return r, fmt.Errorf("%w: amplitude amendment missing new_amplitude", ErrInvalidInput)
		}
		applyRes, aerr = tx.ExecContext(ctx,
			`UPDATE constitution SET content=?, amplitude=? WHERE id=? AND deleted_at IS NULL`,
			tr.content, newAmp.Float64, targetID)
	case KindDelete:
		applyRes, aerr = tx.ExecContext(ctx,
			`UPDATE constitution SET deleted_at=?, deleted_by=? WHERE id=? AND deleted_at IS NULL`,
			now, "amend-approved:"+voter, targetID)
	default:
		return r, fmt.Errorf("%w: unknown amendment kind %q", ErrInvalidInput, kind)
	}
	if aerr != nil {
		return r, fmt.Errorf("apply %s: %w", kind, aerr)
	}
	if n, _ := applyRes.RowsAffected(); n == 0 {
		return r, fmt.Errorf("apply %s: target rule id %d not updated (0 rows)", kind, targetID)
	}

	if _, uerr := tx.ExecContext(ctx,
		`UPDATE constitution_amendment SET status='approved', applied=1, decided_at=?, decided_by=?
		 WHERE id=? AND status='pending'`, now, voter, opts.AmendmentID,
	); uerr != nil {
		return r, fmt.Errorf("mark settled: %w", uerr)
	}
	if cerr := tx.Commit(); cerr != nil {
		return r, fmt.Errorf("commit: %w", cerr)
	}
	committed = true
	r.Status = "approved"
	r.Applied = true
	return r, nil
}

func CountPendingAmendments(ctx context.Context) (int64, error) {
	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	if err := ensureAmendSchema(ctx, db); err != nil {
		return 0, err
	}
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM constitution_amendment WHERE status='pending'`,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
