// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 10 (Tool invocation audit log) phase 1 DONE.
//   API stable: LogToolInvocation (8KB cap args/result/error),
//   ListToolInvocations (tool_name/caller filter, cap 500),
//   CountToolInvocations. Section 8 Retention already handles prune
//   (cron sweep). Phase 2 host capability `host_log_tool_invocation`
//   buat WASM agent panggil dari sandbox → tambah file baru, JANGAN
//   modify ini.
//
// tool_invocations.go — Section 10 phase 1: tool call audit log per-warga.
//
// PURPOSE:
//   Catat setiap tool call ke `tool_invocations` table. Buat audit
//   (siapa pakai tool apa kapan, latency, error). Anti over-prompt:
//   JANGAN auto-inject ke chat context — akses cuma via endpoint
//   dashboard atau retention prune (Section 8 sudah handle).
//
// SCHEMA REUSE:
//   Table `tool_invocations` (di ensureSchema): id, tool_name, args_json,
//   result_json, error_text, latency_ms, caller, invoked_at, deleted_at.
//
// USE CASES:
//   - Audit: tool X di-call N kali, P% fail, avg latency Y ms.
//   - Training: filter status=success export ke JSONL.
//   - Debug: cari invocation error terakhir untuk tool tertentu.
//
// ⚠️ args_json + result_json HARD CAP 8KB each anti-bloat.

package agentdb

import (
	"database/sql"
	"fmt"
	"time"
)

// ToolInvocation — single row.
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

// LogToolInvocation — insert single row. Required: tool_name. Args/result
// JSON strings expected pre-marshaled (caller pakai tools.MarshalArgs).
// Hard cap 8KB each.
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

// ListToolInvocations — paginated. Filter optional tool_name + caller.
// Order: invoked_at DESC. Default 50, max 500.
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

// CountToolInvocations — non-deleted total, optional filter tool_name.
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
