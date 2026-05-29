// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 1 (Tool execution sandbox). API stable:
//   SandboxRun wraps Tool.Run dengan 3 gate (capability, disabled,
//   rate_limit). ErrSandbox* sentinels supaya caller bisa branch.
//   Phase 2 (interceptors, hooks, deadline contextual) → tambah file
//   baru, JANGAN modify ini.
//
// sandbox.go — Section 12 phase 1: tool execution sandbox.
//
// 3 gate per-tool sebelum Run:
//   1. Capability gate: kalau ctx has CapsChecker, cek Tool.Capability()
//      against broker IsApproved. Empty capability = allow (no-cap tools).
//   2. Disabled gate: kalau tool_overrides.disabled=1 di state.db, reject.
//   3. Rate limit gate: kalau tool_overrides.rate_limit > 0, cek
//      invocations per tool_name di last 60s. Reject kalau exceed.

package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors — caller branch.
var (
	ErrSandboxCapDenied   = errors.New("sandbox: capability denied")
	ErrSandboxDisabled    = errors.New("sandbox: tool disabled per agent override")
	ErrSandboxRateLimited = errors.New("sandbox: rate limit exceeded")
)

// SandboxOpts — opsi gate. Phase 1: default-allow kalau callback nil.
type SandboxOpts struct {
	// SkipCapGate — explicit bypass cap check (untuk admin endpoint test).
	SkipCapGate bool
	// SkipDisabledGate — bypass tool_overrides disabled flag.
	SkipDisabledGate bool
	// SkipRateLimit — bypass rate limit check.
	SkipRateLimit bool
}

// SandboxRun — gate + delegate ke Tool.Run. Caller (dispatcher) panggil
// SandboxRun instead of t.Run langsung untuk Section 12 enforcement.
//
// Phase 1 default-allow kalau ctx ngga punya CapsChecker / store —
// backward compat dengan endpoint admin yang masih in-progress wiring.
func SandboxRun(ctx context.Context, t Tool, args map[string]any, opts SandboxOpts) (Result, error) {
	name := t.Name()

	// Gate 1: capability check.
	if !opts.SkipCapGate {
		if check := FromCapsChecker(ctx); check != nil {
			cap := t.Capability()
			if cap != "" && !check(cap) {
				return Result{}, fmt.Errorf("%w: %s requires %q", ErrSandboxCapDenied, name, cap)
			}
		}
	}

	// Gate 2: disabled flag (state.db tool_overrides). Open store via ctx.
	storeReader, hasStore := FromStore(ctx)
	if !opts.SkipDisabledGate && hasStore {
		if disabled, derr := isToolDisabled(storeReader.DB(), name); derr == nil && disabled {
			return Result{}, fmt.Errorf("%w: %s", ErrSandboxDisabled, name)
		}
	}

	// Gate 3: rate limit (per-minute window via invocation log count).
	if !opts.SkipRateLimit && hasStore {
		limit, lerr := toolRateLimit(storeReader.DB(), name)
		if lerr == nil && limit > 0 {
			count, cerr := countRecentInvocations(storeReader.DB(), name, 60*time.Second)
			if cerr == nil && count >= limit {
				return Result{}, fmt.Errorf("%w: %s exceeded %d/min (count=%d)",
					ErrSandboxRateLimited, name, limit, count)
			}
		}
	}

	return t.Run(ctx, args)
}

// Helper queries — keep di sini supaya sandbox self-contained.
//
// Catatan: FromStore mengembalikan *agentdb.Store, kita perlu akses ke
// db langsung. Tambah Store.DB() exporter — tapi mungkin udah ada? Lihat
// notes di tools/db_accessor.go (phase 2 tambah accessor kalau perlu).
//
// Phase 1: query langsung pakai SQL via dummy interface. Sebenernya butuh
// akses ke *sql.DB — exposed dari agentdb via Section 12 helper di
// agentdb package (`DB()` method tambahkan).

func isToolDisabled(db *sql.DB, toolName string) (bool, error) {
	var disabled int
	err := db.QueryRow(
		`SELECT disabled FROM tool_overrides WHERE tool_name = ?`,
		toolName,
	).Scan(&disabled)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return disabled != 0, nil
}

func toolRateLimit(db *sql.DB, toolName string) (int64, error) {
	var limit int64
	err := db.QueryRow(
		`SELECT rate_limit FROM tool_overrides WHERE tool_name = ?`,
		toolName,
	).Scan(&limit)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return limit, nil
}

func countRecentInvocations(db *sql.DB, toolName string, window time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-window).Format(time.RFC3339)
	var n int64
	err := db.QueryRow(
		`SELECT COUNT(*) FROM tool_invocations
		 WHERE tool_name = ? AND invoked_at >= ? AND deleted_at IS NULL`,
		toolName, cutoff,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}
