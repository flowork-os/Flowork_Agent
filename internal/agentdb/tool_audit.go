// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 3 — tool_audit (acceptance criteria spec) +
//   approval_queue (manual approve session). Append-only enforced via
//   Go API. Phase 4+ (hash chain immutability, distributed audit
//   forwarding) → tambah file baru.
//
// tool_audit.go — Section 12 phase 3: tool execution audit + approval queue.

package agentdb

import (
	"fmt"
	"time"
)

// ToolAudit — per Section 12 acceptance criteria. Logged setiap Sandbox
// Run, sukses atau fail.
type ToolAudit struct {
	ID         int64  `json:"id"`
	ToolName   string `json:"tool_name"`
	Decision   string `json:"decision"`   // 'allowed' | 'denied_cap' | 'denied_disabled' | 'denied_rate' | 'denied_interceptor' | 'denied_protector' | 'pending_approve'
	Reason     string `json:"reason"`
	ArgsHash   string `json:"args_hash"`  // SHA256 hex of args JSON
	Caller     string `json:"caller"`
	OccurredAt string `json:"occurred_at"`
}

// ApprovalQueueRow — pending sensitive operation menunggu owner approve.
type ApprovalQueueRow struct {
	ID          int64  `json:"id"`
	ToolName    string `json:"tool_name"`
	ArgsJSON    string `json:"args_json"`
	ArgsHash    string `json:"args_hash"`
	Reason      string `json:"reason"`
	Status      string `json:"status"` // 'pending' | 'approved' | 'rejected' | 'expired'
	Caller      string `json:"caller"`
	RequestedAt string `json:"requested_at"`
	DecidedAt   string `json:"decided_at"`
	DecidedBy   string `json:"decided_by"`
}

func (s *Store) ensureToolAuditSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tool_audit (
		  id          INTEGER PRIMARY KEY AUTOINCREMENT,
		  tool_name   TEXT NOT NULL,
		  decision    TEXT NOT NULL,
		  reason      TEXT NOT NULL DEFAULT '',
		  args_hash   TEXT NOT NULL DEFAULT '',
		  caller      TEXT NOT NULL DEFAULT '',
		  occurred_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_tool_audit_time ON tool_audit(occurred_at DESC);
		CREATE INDEX IF NOT EXISTS idx_tool_audit_decision ON tool_audit(decision);
		CREATE INDEX IF NOT EXISTS idx_tool_audit_tool ON tool_audit(tool_name);

		CREATE TABLE IF NOT EXISTS approval_queue (
		  id           INTEGER PRIMARY KEY AUTOINCREMENT,
		  tool_name    TEXT NOT NULL,
		  args_json    TEXT NOT NULL DEFAULT '{}',
		  args_hash    TEXT NOT NULL DEFAULT '',
		  reason       TEXT NOT NULL DEFAULT '',
		  status       TEXT NOT NULL DEFAULT 'pending',
		  caller       TEXT NOT NULL DEFAULT '',
		  requested_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		  decided_at   TEXT,
		  decided_by   TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_approval_queue_status ON approval_queue(status);
		CREATE INDEX IF NOT EXISTS idx_approval_queue_hash ON approval_queue(args_hash);
	`)
	return err
}

// AppendToolAudit append-only. Auto-stamp occurred_at.
func (s *Store) AppendToolAudit(a ToolAudit) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolAuditSchema(); err != nil {
		return 0, err
	}
	if a.ToolName == "" {
		return 0, fmt.Errorf("tool_name required")
	}
	if a.Decision == "" {
		a.Decision = "allowed"
	}
	if a.OccurredAt == "" {
		a.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.Exec(
		`INSERT INTO tool_audit (tool_name, decision, reason, args_hash, caller, occurred_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		a.ToolName, a.Decision, a.Reason, a.ArgsHash, a.Caller, a.OccurredAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListToolAudit paginated.
func (s *Store) ListToolAudit(decision, toolName string, limit int) ([]ToolAudit, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolAuditSchema(); err != nil {
		return nil, err
	}
	query := `SELECT id, tool_name, decision, reason, args_hash, caller, occurred_at
	          FROM tool_audit WHERE 1=1`
	args := []any{}
	if decision != "" {
		query += ` AND decision = ?`
		args = append(args, decision)
	}
	if toolName != "" {
		query += ` AND tool_name = ?`
		args = append(args, toolName)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ToolAudit{}
	for rows.Next() {
		var a ToolAudit
		if serr := rows.Scan(&a.ID, &a.ToolName, &a.Decision, &a.Reason,
			&a.ArgsHash, &a.Caller, &a.OccurredAt); serr != nil {
			return nil, serr
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// =============================================================================
// Approval queue: manual approve session workflow
// =============================================================================

// EnqueueApproval — insert pending row. Return ID.
func (s *Store) EnqueueApproval(a ApprovalQueueRow) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolAuditSchema(); err != nil {
		return 0, err
	}
	if a.ToolName == "" {
		return 0, fmt.Errorf("tool_name required")
	}
	if a.Status == "" {
		a.Status = "pending"
	}
	if a.RequestedAt == "" {
		a.RequestedAt = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := s.db.Exec(
		`INSERT INTO approval_queue (tool_name, args_json, args_hash, reason, status, caller, requested_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ToolName, a.ArgsJSON, a.ArgsHash, a.Reason, a.Status, a.Caller, a.RequestedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CheckApprovalByHash — return true kalau ada row dengan args_hash + status=approved
// dalam window 1 jam (session-level). Caller (sandbox) pakai untuk re-check
// kalau retry tool yang sebelumnya pending.
func (s *Store) CheckApprovalByHash(toolName, argsHash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolAuditSchema(); err != nil {
		return false, err
	}
	cutoff := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM approval_queue
		 WHERE tool_name = ? AND args_hash = ?
		   AND status = 'approved' AND decided_at >= ?`,
		toolName, argsHash, cutoff,
	).Scan(&n)
	return n > 0, err
}

// DecideApproval — set status approved/rejected + decided_at + decided_by.
func (s *Store) DecideApproval(id int64, status, decidedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolAuditSchema(); err != nil {
		return err
	}
	if status != "approved" && status != "rejected" {
		return fmt.Errorf("invalid status %q (use approved or rejected)", status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE approval_queue
		 SET status = ?, decided_at = ?, decided_by = ?
		 WHERE id = ? AND status = 'pending'`,
		status, now, decidedBy, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("approval %d not found or already decided", id)
	}
	return nil
}

// ListApprovalQueue paginated.
func (s *Store) ListApprovalQueue(status string, limit int) ([]ApprovalQueueRow, error) {
	if limit <= 0 {
		limit = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureToolAuditSchema(); err != nil {
		return nil, err
	}
	query := `SELECT id, tool_name, args_json, args_hash, reason, status,
	                 caller, requested_at,
	                 COALESCE(decided_at, ''), decided_by
	          FROM approval_queue WHERE 1=1`
	args := []any{}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ApprovalQueueRow{}
	for rows.Next() {
		var a ApprovalQueueRow
		if serr := rows.Scan(&a.ID, &a.ToolName, &a.ArgsJSON, &a.ArgsHash,
			&a.Reason, &a.Status, &a.Caller, &a.RequestedAt,
			&a.DecidedAt, &a.DecidedBy); serr != nil {
			return nil, serr
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
