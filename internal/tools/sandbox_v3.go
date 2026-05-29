// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
//
// Locked at: 2026-05-30
// Reason: Section 12 phase 3 — SandboxRunV3 wraps V2 with:
//   1. tool_audit append (every Run, success or fail)
//   2. approval queue check (sensitive ops yang udah ada approved row →
//      pass; baru → enqueue pending + ErrPendingApprove)
//   3. args hash (SHA256) for replay protection + approval matching
//   Locked V2 ngga di-modify. Caller (agentmgr ToolRunHandler) panggil
//   V3. Phase 4+ (cryptographic chain, multi-signer) → tambah file baru.
//
// sandbox_v3.go — Section 12 phase 3: audit + approval workflow.

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"flowork-gui/internal/agentdb"
)

// SensitiveSubstrings — substring di args.path / args.command yg perlu
// approve session. Mirror referensifile section 12 doctrine: state.db
// direct write + sudo + chmod 777.
var sensitiveSubstrings = []string{
	"state.db",       // agent DB direct write
	"/etc/sudoers",   // sudoers modification
	"/etc/passwd",    // passwd write (read is blocked by workspace interceptor)
}

// ErrPendingApprove — sentinel buat caller (handler) branch ke approval
// workflow. Body wajib include queue_id supaya user bisa approve.
var ErrPendingApprove = errors.New("sandbox: pending owner approval")

// SandboxRunV3 — wrapper V2 dengan tool_audit + approval queue.
//
// Flow:
//
//   1. Hash args → sha256 hex.
//   2. Cek pattern sensitif di string args. Kalau hit:
//      a. Cek approval queue: udah ada approved (status='approved',
//         decided_at < 1h ago, same args_hash + tool_name) → pass-through.
//      b. Belum ada approved → enqueue 'pending' row + AppendToolAudit
//         decision='pending_approve' → return ErrPendingApprove dengan
//         queue ID di error message.
//   3. Tidak sensitif → delegate ke SandboxRunV2.
//   4. Setelah V2 return:
//      a. Sukses → AppendToolAudit decision='allowed'.
//      b. Error sandbox → AppendToolAudit decision sesuai sentinel
//         (denied_cap / denied_disabled / denied_rate / denied_interceptor).
//
// Note: store wajib ada di ctx (via WithStore di dispatcher).
func SandboxRunV3(ctx context.Context, t Tool, args map[string]any, opts SandboxOpts) (Result, error) {
	toolName := t.Name()
	caller := FromCaller(ctx)
	argsHash := hashArgs(args)

	store, hasStore := FromStore(ctx)
	if !hasStore {
		// Without store, fall back to V2 (degraded mode — ngga ada audit).
		return SandboxRunV2(ctx, t, args, opts)
	}

	// Sensitive pattern check.
	if isSensitiveArgs(args) {
		// Check approval queue.
		approved, err := store.CheckApprovalByHash(toolName, argsHash)
		if err == nil && approved {
			// Pass-through. Record audit + delegate.
			_, _ = store.AppendToolAudit(agentdb.ToolAudit{
				ToolName: toolName,
				Decision: "allowed",
				Reason:   "approved via approval_queue (session-1h)",
				ArgsHash: argsHash,
				Caller:   caller,
			})
		} else {
			// Enqueue pending + record audit.
			argsJSON, _ := json.Marshal(args)
			queueID, qerr := store.EnqueueApproval(agentdb.ApprovalQueueRow{
				ToolName: toolName,
				ArgsJSON: string(argsJSON),
				ArgsHash: argsHash,
				Reason:   sensitiveReason(args),
				Status:   "pending",
				Caller:   caller,
			})
			if qerr != nil {
				return Result{}, fmt.Errorf("enqueue approval: %w", qerr)
			}
			_, _ = store.AppendToolAudit(agentdb.ToolAudit{
				ToolName: toolName,
				Decision: "pending_approve",
				Reason:   fmt.Sprintf("queue_id=%d", queueID),
				ArgsHash: argsHash,
				Caller:   caller,
			})
			return Result{}, fmt.Errorf("%w: queue_id=%d (POST /api/agents/protector/approve_pending?queue_id=%d to allow)",
				ErrPendingApprove, queueID, queueID)
		}
	}

	// Delegate ke V2.
	res, runErr := SandboxRunV2(ctx, t, args, opts)

	// Audit decision.
	decision := "allowed"
	reason := ""
	if runErr != nil {
		decision = mapErrToDecision(runErr)
		reason = runErr.Error()
		if len(reason) > 512 {
			reason = reason[:512] + "…"
		}
	}
	_, _ = store.AppendToolAudit(agentdb.ToolAudit{
		ToolName: toolName,
		Decision: decision,
		Reason:   reason,
		ArgsHash: argsHash,
		Caller:   caller,
	})
	// Section 26 phase 2 auto-hook: also append to unified audit_log
	// (event_type=tool_call) untuk watchdog rule evaluator single stream.
	sev := agentdb.AuditSevInfo
	if decision == "denied_interceptor" || decision == "denied_cap" || decision == "pending_approve" {
		sev = agentdb.AuditSevWarning
	}
	if runErr != nil && decision == "error" {
		sev = agentdb.AuditSevError
	}
	detail, _ := json.Marshal(map[string]any{
		"tool":      toolName,
		"decision":  decision,
		"args_hash": argsHash,
		"caller":    caller,
		"reason":    reason,
	})
	_, _ = store.AppendAudit(agentdb.AuditEntry{
		EventType:  agentdb.EventToolCall,
		Severity:   sev,
		Actor:      caller,
		DetailJSON: string(detail),
	})
	// Also append event_type=protector_block kalau interceptor-blocked.
	if decision == "denied_interceptor" {
		_, _ = store.AppendAudit(agentdb.AuditEntry{
			EventType:  agentdb.EventProtectorBlock,
			Severity:   agentdb.AuditSevWarning,
			Actor:      caller,
			DetailJSON: string(detail),
		})
	}
	return res, runErr
}

// hashArgs — deterministic JSON marshal (Go maps not ordered, but JSON
// produces stable serialization for nested maps karena Go 1.12+).
func hashArgs(args map[string]any) string {
	b, _ := json.Marshal(args)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func isSensitiveArgs(args map[string]any) bool {
	for _, v := range args {
		if s, ok := v.(string); ok {
			ls := strings.ToLower(s)
			for _, p := range sensitiveSubstrings {
				if strings.Contains(ls, strings.ToLower(p)) {
					return true
				}
			}
		}
	}
	return false
}

func sensitiveReason(args map[string]any) string {
	for _, v := range args {
		if s, ok := v.(string); ok {
			ls := strings.ToLower(s)
			for _, p := range sensitiveSubstrings {
				if strings.Contains(ls, strings.ToLower(p)) {
					return "sensitive substring detected: " + p
				}
			}
		}
	}
	return "sensitive op"
}

func mapErrToDecision(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "capability denied"):
		return "denied_cap"
	case strings.Contains(msg, "tool disabled"):
		return "denied_disabled"
	case strings.Contains(msg, "rate limit exceeded"):
		return "denied_rate"
	case strings.Contains(msg, "interceptor blocked"):
		return "denied_interceptor"
	case strings.Contains(msg, "pending owner approval"):
		return "pending_approve"
	}
	return "error"
}
