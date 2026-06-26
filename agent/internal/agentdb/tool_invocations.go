// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

type ToolInvocation struct {
	ID         int64  `json:"id"`
	ToolName   string `json:"tool_name"`
	ArgsJSON   string `json:"args_json"`
	ResultJSON string `json:"result_json"`
	ErrorText  string `json:"error_text,omitempty"`
	LatencyMs  int64  `json:"latency_ms"`
	Caller     string `json:"caller"`
	InvokedAt  string `json:"invoked_at"`
}

func (s *Store) LogToolInvocation(toolName, argsJSON, resultJSON, errorText, caller string, latencyMs int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if toolName == "" {
		return 0, fmt.Errorf("tool_name required")
	}
	const maxBytes = 8 * 1024
	if len(argsJSON) > maxBytes {
		argsJSON = argsJSON[:maxBytes] + "…[truncated]"
	}
	if len(resultJSON) > maxBytes {
		resultJSON = resultJSON[:maxBytes] + "…[truncated]"
	}
	if len(errorText) > maxBytes {
		errorText = errorText[:maxBytes] + "…"
	}
	if argsJSON == "" {
		argsJSON = "{}"
	}
	if resultJSON == "" {
		resultJSON = "{}"
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO tool_invocations(tool_name, args_json, result_json, error_text,
		                              latency_ms, caller, invoked_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		toolName, argsJSON, resultJSON, errorText, latencyMs, caller, ts,
	)
	if err != nil {
		return 0, fmt.Errorf("insert invocation: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) ListToolInvocations(toolName, caller string, limit int) ([]ToolInvocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	query := `SELECT id, tool_name, args_json, result_json, error_text,
	                 latency_ms, caller, invoked_at
	          FROM tool_invocations WHERE deleted_at IS NULL`
	args := []any{}
	if toolName != "" {
		query += ` AND tool_name = ?`
		args = append(args, toolName)
	}
	if caller != "" {
		query += ` AND caller = ?`
		args = append(args, caller)
	}
	query += ` ORDER BY invoked_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query invocations: %w", err)
	}
	defer rows.Close()

	var out []ToolInvocation
	for rows.Next() {
		var t ToolInvocation
		if err := rows.Scan(&t.ID, &t.ToolName, &t.ArgsJSON, &t.ResultJSON,
			&t.ErrorText, &t.LatencyMs, &t.Caller, &t.InvokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CountToolInvocations(toolName string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `SELECT COUNT(*) FROM tool_invocations WHERE deleted_at IS NULL`
	args := []any{}
	if toolName != "" {
		query += ` AND tool_name = ?`
		args = append(args, toolName)
	}
	var n int64
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}
