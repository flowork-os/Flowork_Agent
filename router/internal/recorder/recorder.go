// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package recorder

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

const AlgoVersion = "v1"

type Record struct {
	ID           int64   `json:"id"`
	PromptHash   string  `json:"prompt_hash"`
	Model        string  `json:"model"`
	Prompt       string  `json:"prompt,omitempty"`
	Response     string  `json:"response,omitempty"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	BuildPass    int64   `json:"build_pass"`
	ToolCalls    string  `json:"tool_calls"`
	Agent        string  `json:"agent"`
	CreatedAt    string  `json:"created_at"`
}

type RecordOpts struct {
	Model        string
	RequestBody  any
	ResponseText string
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
	BuildPass    int64
	ToolCalls    []any
	Agent        string
}

func Save(ctx context.Context, opts RecordOpts) (int64, error) {
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		return 0, fmt.Errorf("model required")
	}

	const maxBytes = 32 * 1024
	respText := opts.ResponseText
	if len(respText) > maxBytes {
		respText = respText[:maxBytes] + "…[truncated]"
	}

	var prompt string
	if opts.RequestBody != nil {
		b, err := json.Marshal(opts.RequestBody)
		if err != nil {
			return 0, fmt.Errorf("marshal request: %w", err)
		}
		if len(b) > maxBytes {
			prompt = string(b[:maxBytes]) + "…[truncated]"
		} else {
			prompt = string(b)
		}
	} else {
		prompt = "{}"
	}

	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])[:16]

	var toolCallsJSON string = "[]"
	if len(opts.ToolCalls) > 0 {
		if b, err := json.Marshal(opts.ToolCalls); err == nil {
			if len(b) > maxBytes {
				toolCallsJSON = string(b[:maxBytes]) + "…[truncated]"
			} else {
				toolCallsJSON = string(b)
			}
		}
	}

	buildPass := opts.BuildPass
	if buildPass == 0 && opts.BuildPass != 0 {

	}

	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx,
		`INSERT INTO recordings(prompt_hash, prompt, response, model,
		                        input_tokens, output_tokens, cost_usd,
		                        build_pass, tool_calls, agent)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		promptHash, prompt, respText, model,
		opts.InputTokens, opts.OutputTokens, opts.CostUSD,
		buildPass, toolCallsJSON, opts.Agent,
	)
	if err != nil {
		return 0, fmt.Errorf("insert recording: %w", err)
	}
	return res.LastInsertId()
}

type ListOpts struct {
	Model       string
	Agent       string
	Limit       int
	IncludeBody bool
}

func List(ctx context.Context, opts ListOpts) ([]Record, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	db, err := brain.OpenRW()
	if err != nil {
		return nil, err
	}

	cols := "id, prompt_hash, model, input_tokens, output_tokens, cost_usd, build_pass, tool_calls, agent, created_at"
	if opts.IncludeBody {
		cols = "id, prompt_hash, model, prompt, response, input_tokens, output_tokens, cost_usd, build_pass, tool_calls, agent, created_at"
	}

	query := `SELECT ` + cols + ` FROM recordings WHERE deleted_at IS NULL`
	args := []any{}
	if opts.Model != "" {
		query += ` AND model = ?`
		args = append(args, opts.Model)
	}
	if opts.Agent != "" {
		query += ` AND agent = ?`
		args = append(args, opts.Agent)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, qerr := db.QueryContext(ctx, query, args...)
	if qerr != nil {
		return nil, fmt.Errorf("query recordings: %w", qerr)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		if opts.IncludeBody {
			if err := rows.Scan(&r.ID, &r.PromptHash, &r.Model,
				&r.Prompt, &r.Response,
				&r.InputTokens, &r.OutputTokens, &r.CostUSD,
				&r.BuildPass, &r.ToolCalls, &r.Agent, &r.CreatedAt); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&r.ID, &r.PromptHash, &r.Model,
				&r.InputTokens, &r.OutputTokens, &r.CostUSD,
				&r.BuildPass, &r.ToolCalls, &r.Agent, &r.CreatedAt); err != nil {
				return nil, err
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func Get(ctx context.Context, id int64) (Record, error) {
	if id <= 0 {
		return Record{}, fmt.Errorf("id required")
	}
	db, err := brain.OpenRW()
	if err != nil {
		return Record{}, err
	}
	var r Record
	rerr := db.QueryRowContext(ctx,
		`SELECT id, prompt_hash, model, prompt, response,
		        input_tokens, output_tokens, cost_usd,
		        build_pass, tool_calls, agent, created_at
		 FROM recordings WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&r.ID, &r.PromptHash, &r.Model, &r.Prompt, &r.Response,
		&r.InputTokens, &r.OutputTokens, &r.CostUSD,
		&r.BuildPass, &r.ToolCalls, &r.Agent, &r.CreatedAt)
	if rerr == sql.ErrNoRows {
		return Record{ID: id}, nil
	}
	if rerr != nil {
		return Record{}, fmt.Errorf("get recording: %w", rerr)
	}
	return r, nil
}

func Count(ctx context.Context) (int64, error) {
	db, err := brain.OpenRW()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM recordings WHERE deleted_at IS NULL`,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
