// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const ToolLearnerAlgoVersion = "v1"

type ToolPattern struct {
	ID             int64   `json:"id"`
	TriggerPattern string  `json:"trigger_pattern"`
	ToolName       string  `json:"tool_name"`
	SuccessCount   int64   `json:"success_count"`
	FailCount      int64   `json:"fail_count"`
	Amplitude      float64 `json:"amplitude"`
}

func LearnPattern(ctx context.Context, trigger, toolName string, success bool) (float64, error) {
	trigger = strings.TrimSpace(trigger)
	toolName = strings.TrimSpace(toolName)
	if trigger == "" || toolName == "" {
		return 0, fmt.Errorf("trigger + tool_name required")
	}
	const maxLen = 256
	if len(trigger) > maxLen {
		trigger = trigger[:maxLen]
	}
	if len(toolName) > maxLen {
		toolName = toolName[:maxLen]
	}

	db, err := OpenRW()
	if err != nil {
		return 0, err
	}

	successDelta := 0
	failDelta := 0
	if success {
		successDelta = 1
	} else {
		failDelta = 1
	}

	var amplitude float64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO tool_patterns(trigger_pattern, tool_name, success_count, fail_count, amplitude)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(trigger_pattern, tool_name) DO UPDATE SET
		     success_count = tool_patterns.success_count + excluded.success_count,
		     fail_count    = tool_patterns.fail_count + excluded.fail_count,
		     amplitude     = CAST(tool_patterns.success_count + excluded.success_count AS REAL) /
		                     MAX(1, tool_patterns.success_count + tool_patterns.fail_count + excluded.success_count + excluded.fail_count),
		     deleted_at    = NULL,
		     deleted_by    = NULL
		 RETURNING amplitude`,
		trigger, toolName, successDelta, failDelta, computeInitialAmplitude(successDelta, failDelta),
	).Scan(&amplitude); err != nil {
		return 0, fmt.Errorf("upsert pattern: %w", err)
	}
	return amplitude, nil
}

func computeInitialAmplitude(success, fail int) float64 {
	total := success + fail
	if total == 0 {
		return 0.5
	}
	return float64(success) / float64(total)
}

func SuggestTools(ctx context.Context, trigger string, limit int) ([]ToolPattern, error) {
	trigger = strings.TrimSpace(trigger)
	if trigger == "" {
		return nil, fmt.Errorf("trigger required")
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}

	db, err := OpenRW()
	if err != nil {
		return nil, err
	}

	rows, qerr := db.QueryContext(ctx,
		`SELECT id, trigger_pattern, tool_name, success_count, fail_count, amplitude
		 FROM tool_patterns
		 WHERE deleted_at IS NULL
		   AND trigger_pattern LIKE ?
		 ORDER BY amplitude DESC, success_count DESC
		 LIMIT ?`,
		"%"+trigger+"%", limit,
	)
	if qerr != nil {
		return nil, fmt.Errorf("query tool patterns: %w", qerr)
	}
	defer rows.Close()

	var out []ToolPattern
	for rows.Next() {
		var p ToolPattern
		if err := rows.Scan(&p.ID, &p.TriggerPattern, &p.ToolName,
			&p.SuccessCount, &p.FailCount, &p.Amplitude); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func CountToolPatterns(ctx context.Context) (int64, error) {
	db, err := OpenRW()
	if err != nil {
		return 0, err
	}
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tool_patterns WHERE deleted_at IS NULL`,
	).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
