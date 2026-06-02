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
	"os"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/protector"
)

// SensitiveSubstrings — substring di args.path / args.command yg perlu
// approve session. Mirror referensifile section 12 doctrine: state.db
// direct write + sudo + chmod 777.
var sensitiveSubstrings = []string{
	"state.db",     // agent DB direct write
	"/etc/sudoers", // sudoers modification
	"/etc/passwd",  // passwd write (read is blocked by workspace interceptor)
}

// sensitiveTools — tools that ALWAYS require explicit owner approval, regardless
// of args. An AI agent must not power off / reboot / lock the host without an
// out-of-band owner confirmation (the in-chat "yakin?" can be socially engineered;
// the approval queue cannot). Pairs with the system_power ARM switch.
var sensitiveTools = map[string]bool{
	"system_power": true,
}

func isSensitiveTool(name string) bool {
	if !sensitiveTools[name] {
		return false
	}
	// Power control is destructive; the owner can require an out-of-band approval
	// (GUI approval queue) for every real action by setting this env. Default off
	// preserves the one-step chat→action flow (the exec:power cap + ARM switch +
	// caller-binding already gate who may invoke it).
	if name == "system_power" {
		return os.Getenv("FLOWORK_POWER_REQUIRE_APPROVAL") == "1"
	}
	return true
}

// protectorBlockHit applies the IMMUTABLE Host Protection Gate baseline
// (protector.Baseline) on the actual tool-execution path — not just the UI test
// endpoint. To avoid false-positives on content tools (a drawer whose TEXT
// mentions /etc/passwd must not be blocked), it only inspects args that are
// genuinely commands / paths / hosts (by key name). Returns the blocking rule.
func protectorBlockHit(args map[string]any) (bool, string) {
	hit := func(ruleType, val string) (bool, string) {
		if strings.TrimSpace(val) == "" {
			return false, ""
		}
		if rule, ok := protector.CheckPattern(ruleType, val, nil); ok && rule.Action == protector.ActionBlock {
			return true, rule.Type + ":" + rule.Pattern
		}
		return false, ""
	}
	for k, v := range args {
		s, ok := v.(string)
		if !ok {
			continue
		}
		lk := strings.ToLower(k)
		switch {
		case lk == "command" || lk == "cmd" || lk == "script":
			if b, r := hit(protector.TypeCommand, s); b {
				return true, r
			}
		case lk == "url" || lk == "host" || lk == "ip" || lk == "address":
			if b, r := hit(protector.TypeIP, s); b {
				return true, r
			}
		case lk == "path" || lk == "file" || lk == "dir" || lk == "working_dir" ||
			lk == "name" || lk == "target" || lk == "target_path" || strings.HasSuffix(lk, "_path"):
			if b, r := hit(protector.TypeFilePath, s); b {
				return true, r
			}
		}
	}
	return false, ""
}

// ErrPendingApprove — sentinel buat caller (handler) branch ke approval
// workflow. Body wajib include queue_id supaya user bisa approve.
var ErrPendingApprove = errors.New("sandbox: pending owner approval")

// SandboxRunV3 — wrapper V2 dengan tool_audit + approval queue.
//
// Flow:
//
//  1. Hash args → sha256 hex.
//  2. Cek pattern sensitif di string args. Kalau hit:
//     a. Cek approval queue: udah ada approved (status='approved',
//     decided_at < 1h ago, same args_hash + tool_name) → pass-through.
//     b. Belum ada approved → enqueue 'pending' row + AppendToolAudit
//     decision='pending_approve' → return ErrPendingApprove dengan
//     queue ID di error message.
//  3. Tidak sensitif → delegate ke SandboxRunV2.
//  4. Setelah V2 return:
//     a. Sukses → AppendToolAudit decision='allowed'.
//     b. Error sandbox → AppendToolAudit decision sesuai sentinel
//     (denied_cap / denied_disabled / denied_rate / denied_interceptor).
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

	// Host Protection Gate (immutable baseline) — hard-deny destructive ops
	// (rm -rf /, /etc/shadow, sudo, cloud-metadata IP, …) on the real execution
	// path, before approval/delegate. The baseline is owner-immutable.
	if blocked, rule := protectorBlockHit(args); blocked {
		_, _ = store.AppendToolAudit(agentdb.ToolAudit{
			ToolName: toolName,
			Decision: "denied_protector",
			Reason:   "host protection gate: " + rule,
			ArgsHash: argsHash,
			Caller:   caller,
		})
		return Result{}, fmt.Errorf("sandbox: blocked by host protection gate (%s)", rule)
	}

	// Sensitive pattern check (sensitive tool name OR sensitive args).
	if isSensitiveTool(toolName) || isSensitiveArgs(args) {
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
				Reason:   sensitiveReason(toolName, args),
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

func sensitiveReason(toolName string, args map[string]any) string {
	if isSensitiveTool(toolName) {
		return "sensitive tool requires owner approval: " + toolName
	}
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
