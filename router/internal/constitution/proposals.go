// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package constitution

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

const AlgoVersion = "v1"

type Proposal struct {
	ID         int64   `json:"id"`
	SourceFile string  `json:"source_file"`
	Section    string  `json:"section"`
	Content    string  `json:"content,omitempty"`
	Amplitude  float64 `json:"amplitude"`
	CreatedAt  string  `json:"created_at,omitempty"`
}

type ProposeOpts struct {
	SourceFile    string
	Section       string
	Content       string
	Amplitude     float64
	ContextOrigin string
	Signer        string
}

func Propose(ctx context.Context, opts ProposeOpts) (int64, error) {
	sourceFile := strings.TrimSpace(opts.SourceFile)
	section := strings.TrimSpace(opts.Section)
	content := strings.TrimSpace(opts.Content)
	if sourceFile == "" || section == "" || content == "" {
		return 0, fmt.Errorf("source_file + section + content required")
	}
	const (
		maxText  = 16 * 1024
		maxField = 256
	)
	if len(content) > maxText {
		content = content[:maxText] + "…[truncated]"
	}
	if len(sourceFile) > maxField {
		sourceFile = sourceFile[:maxField]
	}
	if len(section) > maxField {
		section = section[:maxField]
	}

	amp := opts.Amplitude
	if amp <= 0 {
		amp = 1.0
	}

	signer := strings.TrimSpace(opts.Signer)
	if signer == "" {
		signer = "anonymous"
	}

	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}

	res, ierr := db.ExecContext(ctx,
		`INSERT INTO constitution(source_file, section, content, amplitude,
		                          pending_quorum_review, context_origin, signer, origin_node)
		 VALUES(?, ?, ?, ?, 1, ?, ?, 'local')`,
		sourceFile, section, content, amp, opts.ContextOrigin, signer,
	)
	if ierr != nil {
		return 0, fmt.Errorf("insert proposal: %w", ierr)
	}
	return res.LastInsertId()
}

func ListPending(ctx context.Context, limit int, includeContent bool) ([]Proposal, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	db, err := brain.OpenRW()
	if err != nil {
		return nil, err
	}

	cols := "id, source_file, section, amplitude"
	if includeContent {
		cols = "id, source_file, section, content, amplitude"
	}

	rows, qerr := db.QueryContext(ctx,
		`SELECT `+cols+`
		 FROM constitution
		 WHERE pending_quorum_review = 1 AND deleted_at IS NULL
		 ORDER BY id ASC LIMIT ?`,
		limit,
	)
	if qerr != nil {
		return nil, fmt.Errorf("query pending: %w", qerr)
	}
	defer rows.Close()

	var out []Proposal
	for rows.Next() {
		var p Proposal
		if includeContent {
			if err := rows.Scan(&p.ID, &p.SourceFile, &p.Section, &p.Content, &p.Amplitude); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&p.ID, &p.SourceFile, &p.Section, &p.Amplitude); err != nil {
				return nil, err
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type VoteOpts struct {
	ProposalID int64
	Action     string
	VoterID    string
}

type VoteResult struct {
	ProposalID int64  `json:"proposal_id"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	VoterID    string `json:"voter_id"`
}

func Vote(ctx context.Context, opts VoteOpts) (VoteResult, error) {
	r := VoteResult{ProposalID: opts.ProposalID, Action: opts.Action, VoterID: opts.VoterID}
	if opts.ProposalID <= 0 {
		return r, fmt.Errorf("proposal_id required")
	}
	action := strings.TrimSpace(opts.Action)
	if action != "approve" && action != "reject" {
		return r, fmt.Errorf("action must be 'approve' or 'reject'")
	}
	voter := strings.TrimSpace(opts.VoterID)
	if voter == "" {
		voter = "anonymous"
	}

	db, err := brain.OpenRW()
	if err != nil {
		return r, err
	}

	var pending int
	var deletedAt sql.NullString
	qerr := db.QueryRowContext(ctx,
		`SELECT pending_quorum_review, deleted_at FROM constitution WHERE id = ?`,
		opts.ProposalID,
	).Scan(&pending, &deletedAt)
	if qerr == sql.ErrNoRows {
		return r, fmt.Errorf("proposal not found")
	}
	if qerr != nil {
		return r, fmt.Errorf("lookup: %w", qerr)
	}
	if deletedAt.Valid {
		r.Status = "no-op"
		return r, nil
	}
	if pending == 0 {

		r.Status = "no-op"
		return r, nil
	}

	switch action {
	case "approve":
		_, uerr := db.ExecContext(ctx,
			`UPDATE constitution SET pending_quorum_review = 0 WHERE id = ?`,
			opts.ProposalID,
		)
		if uerr != nil {
			return r, fmt.Errorf("approve: %w", uerr)
		}
		r.Status = "approved"
	case "reject":
		ts := time.Now().UTC().Format(time.RFC3339)
		deletedBy := "vote-rejected:" + voter
		_, uerr := db.ExecContext(ctx,
			`UPDATE constitution SET deleted_at = ?, deleted_by = ? WHERE id = ?`,
			ts, deletedBy, opts.ProposalID,
		)
		if uerr != nil {
			return r, fmt.Errorf("reject: %w", uerr)
		}
		r.Status = "rejected"
	}
	return r, nil
}

func CountPending(ctx context.Context) (int64, error) {
	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM constitution WHERE pending_quorum_review = 1 AND deleted_at IS NULL`,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
