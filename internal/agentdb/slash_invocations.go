// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 14 phase 1 (slash audit log). API stable:
//   LogSlashInvocation (8KB cap fields), ListSlashInvocations (command/
//   caller filter, cap 500). Retention via Section 8 cron sweep.
//
// slash_invocations.go — Section 14 phase 1: slash command audit log per-warga.

package agentdb

import (
	"fmt"
	"time"
)

// SlashInvocation — single row.
type SlashInvocation struct {
	ID         int64  `json:"id"`
	Command    string `json:"command"`
	Args       string `json:"args"`
	Caller     string `json:"caller"`
	ResultText string `json:"result_text"`
	ErrorText  string `json:"error_text,omitempty"`
	DurationMs int64  `json:"duration_ms"`
	InvokedAt  string `json:"invoked_at"`
}

// LogSlashInvocation — insert single row. Cap fields 8KB anti-bloat.
func (s *Store) LogSlashInvocation(command, args, caller, resultText, errorText string, durationMs int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if command == "" {
		return 0, fmt.Errorf("command required")
	}
	const maxBytes = 8 * 1024
	if len(args) > maxBytes {
		args = args[:maxBytes] + "…"
	}
	if len(resultText) > maxBytes {
		resultText = resultText[:maxBytes] + "…[truncated]"
	}
	if len(errorText) > maxBytes {
		errorText = errorText[:maxBytes] + "…"
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`INSERT INTO slash_invocations(command, args, caller, result_text, error_text,
		                               duration_ms, invoked_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		command, args, caller, resultText, errorText, durationMs, ts,
	)
	if err != nil {
		return 0, fmt.Errorf("insert slash invocation: %w", err)
	}
	return res.LastInsertId()
}

// ListSlashInvocations — paginated. Filter optional command + caller.
// Default 50, max 500.
func (s *Store) ListSlashInvocations(command, caller string, limit int) ([]SlashInvocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > 500 {
		limit = 50
	}
	query := `SELECT id, command, args, caller, result_text, error_text, duration_ms, invoked_at
	          FROM slash_invocations WHERE deleted_at IS NULL`
	args := []any{}
	if command != "" {
		query += ` AND command = ?`
		args = append(args, command)
	}
	if caller != "" {
		query += ` AND caller = ?`
		args = append(args, caller)
	}
	query += ` ORDER BY invoked_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query slash invocations: %w", err)
	}
	defer rows.Close()

	var out []SlashInvocation
	for rows.Next() {
		var si SlashInvocation
		if err := rows.Scan(&si.ID, &si.Command, &si.Args, &si.Caller,
			&si.ResultText, &si.ErrorText, &si.DurationMs, &si.InvokedAt); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}
