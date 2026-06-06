// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 2 — interceptor chain layer on top of sandbox.go
//   (locked). Pattern: SandboxRun (3 gate) → tetap dipakai, di-wrap oleh
//   SandboxRunV2 yang panggil Before(ctx, tool, args) untuk semua
//   registered Interceptor sebelum SandboxRun. Phase 3 (post-hooks,
//   After/AfterError) → tambah file baru, JANGAN modify ini.
//
// interceptors.go — Section 12 phase 2: pre-execution interceptor chain.
//
// Built-in interceptors (registered di Init di package tools/builtins
// via tools.RegisterInterceptor):
//
//   1. workspacePathInterceptor    — block path traversal `..` di string args
//   2. sensitiveFileInterceptor    — block access ke .env / *.key / id_rsa dst.
//   3. personaInjectInterceptor    — strip system-prompt-injection attempt
//                                    dari tool args (e.g. "ignore previous
//                                    instructions", "you are now jailbroken")
//
// Tool builtin BISA bypass interceptor tertentu kalau punya security model
// internal lengkap (mis. bashTool punya denylist sendiri yg lebih ketat
// daripada generic shellInterceptor). Untuk phase 2 simple — semua tool
// lewat semua interceptor.

package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Interceptor — pre-execution hook. Return err non-nil → block run.
type Interceptor interface {
	Name() string
	Before(ctx context.Context, t Tool, args map[string]any) error
}

// Sentinel.
var ErrInterceptorBlocked = errors.New("interceptor blocked")

var (
	interceptorsMu sync.RWMutex
	interceptors   []Interceptor
)

// RegisterInterceptor — append Interceptor ke global chain. Caller (builtins
// Init) panggil exactly once per Interceptor at boot. Idempotent: duplicate
// Name() di-skip diam-diam (anti panic boot).
func RegisterInterceptor(i Interceptor) {
	if i == nil {
		return
	}
	interceptorsMu.Lock()
	defer interceptorsMu.Unlock()
	for _, existing := range interceptors {
		if existing.Name() == i.Name() {
			return
		}
	}
	interceptors = append(interceptors, i)
}

// ListInterceptors — return snapshot slice (debug / admin endpoint).
func ListInterceptors() []string {
	interceptorsMu.RLock()
	defer interceptorsMu.RUnlock()
	out := make([]string, 0, len(interceptors))
	for _, i := range interceptors {
		out = append(out, i.Name())
	}
	return out
}

// SandboxRunV2 — wrap SandboxRun dengan interceptor chain. Order:
//
//   1. Run all registered Interceptor.Before in registration order.
//      First non-nil err → return (wrap dengan ErrInterceptorBlocked).
//   2. Delegate ke SandboxRun (3 gate: cap, disabled, rate_limit).
//
// Caller (dispatcher) panggil SandboxRunV2 instead of SandboxRun untuk
// dapet full pipeline. SandboxRun standalone tetap tersedia kalau ada
// case admin override yg butuh skip interceptor (rare).
func SandboxRunV2(ctx context.Context, t Tool, args map[string]any, opts SandboxOpts) (Result, error) {
	interceptorsMu.RLock()
	chain := make([]Interceptor, len(interceptors))
	copy(chain, interceptors)
	interceptorsMu.RUnlock()

	for _, i := range chain {
		if err := i.Before(ctx, t, args); err != nil {
			return Result{}, fmt.Errorf("%w: %s blocked %s: %w",
				ErrInterceptorBlocked, i.Name(), t.Name(), err)
		}
	}
	return SandboxRun(ctx, t, args, opts)
}

// =============================================================================
// Built-in Interceptor #1: workspacePathInterceptor
// =============================================================================
//
// Scan semua string args yang look-like-path (contain `/` atau `\\` atau
// match common path keys: path, file, name, dir, working_dir, dst).
// Reject kalau:
//   - Contain `..` segment
//   - Absolute path (mulai `/` di Unix atau `C:\` di Windows)
//   - Mengarah ke /etc, /proc, /sys, /root, /home/* (kecuali shared dir)
//
// Allowed: relative path tanpa `..`, OR absolute path yang resolve ke shared dir.

var pathLikeArgKeys = map[string]bool{
	"path":        true,
	"file":        true,
	"filename":    true,
	"filepath":    true,
	"dir":         true,
	"directory":   true,
	"working_dir": true,
	"target":      true,
	"source":      true,
	"src":         true,
	"dest":        true,
	"output":      true,
}

type workspacePathInterceptor struct{}

func (workspacePathInterceptor) Name() string { return "workspace-path" }

func (workspacePathInterceptor) Before(_ context.Context, _ Tool, args map[string]any) error {
	for k, v := range args {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		lk := strings.ToLower(k)
		isPathKey := pathLikeArgKeys[lk]
		looksPathy := strings.ContainsAny(s, "/\\")
		if !isPathKey && !looksPathy {
			continue
		}
		// Defense: reject `..` segment.
		if strings.Contains(s, "..") {
			parts := strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == '\\' })
			for _, p := range parts {
				if p == ".." {
					return fmt.Errorf("path arg %q contains parent traversal '..'", k)
				}
			}
		}
		// Reject absolute paths to dangerous prefixes.
		// (Tool-level resolution handles legitimate abs paths inside shared.)
		dangerous := []string{"/etc/", "/proc/", "/sys/", "/root/", "/var/log/",
			"/.ssh/", "/.aws/", "/.config/secrets",
			"\\Windows\\System32", "\\Users\\Administrator"}
		lower := strings.ToLower(s)
		for _, d := range dangerous {
			if strings.Contains(lower, strings.ToLower(d)) {
				return fmt.Errorf("path arg %q points to protected location %q", k, d)
			}
		}
	}
	return nil
}

// =============================================================================
// Built-in Interceptor #2: sensitiveFileInterceptor
// =============================================================================
//
// Block file-name yang sensitive (credentials, keys, secrets) regardless of
// path. Cross-cut: bahkan kalau attacker resolve path ke shared workspace,
// kalau filename = `.env` atau `id_rsa`, block.

var sensitiveBasenames = map[string]bool{
	".env":             true,
	".env.local":       true,
	".env.production":  true,
	"id_rsa":           true,
	"id_rsa.pub":       true,
	"id_ed25519":       true,
	"id_ed25519.pub":   true,
	"known_hosts":      true,
	"authorized_keys":  true,
	"credentials.json": true,
	"credentials.yaml": true,
	"secrets.yaml":     true,
	"secrets.json":     true,
	".npmrc":           true,
	".pypirc":          true,
	".aws":             true,
	".gnupg":           true,
}

var sensitiveSuffixes = []string{
	".key", ".pem", ".p12", ".pfx", ".jks",
	".token", ".credentials",
}

type sensitiveFileInterceptor struct{}

func (sensitiveFileInterceptor) Name() string { return "sensitive-file" }

func (sensitiveFileInterceptor) Before(_ context.Context, _ Tool, args map[string]any) error {
	check := func(s string) error {
		if s == "" {
			return nil
		}
		// Extract basename without importing filepath (simple last `/` or `\\`).
		base := s
		if i := strings.LastIndexAny(s, "/\\"); i >= 0 {
			base = s[i+1:]
		}
		bl := strings.ToLower(base)
		if sensitiveBasenames[bl] {
			return fmt.Errorf("sensitive file %q blocked", base)
		}
		for _, suf := range sensitiveSuffixes {
			if strings.HasSuffix(bl, suf) {
				return fmt.Errorf("sensitive suffix %q blocked", suf)
			}
		}
		return nil
	}
	for _, v := range args {
		if s, ok := v.(string); ok {
			if err := check(s); err != nil {
				return err
			}
		}
	}
	return nil
}

// =============================================================================
// Built-in Interceptor #3: personaInjectInterceptor
// =============================================================================
//
// Cek string args terhadap pola prompt-injection populer. Bukan exhaustive —
// defense in depth setelah Router promptguard. Lebih ketat di tool input
// karena agent biasanya generate args dari LLM yang udah persona-aware.

var personaInjectionPatterns = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard the above",
	"you are now jailbroken",
	"jailbreak mode",
	"developer mode enabled",
	"system: you are",
	"</system>",
	"<|im_start|>system",
	"forget your instructions",
	"reveal your system prompt",
	"print your instructions",
	"role: system\ncontent:",
	"new instructions:",
}

type personaInjectInterceptor struct{}

func (personaInjectInterceptor) Name() string { return "persona-inject" }

func (personaInjectInterceptor) Before(_ context.Context, _ Tool, args map[string]any) error {
	for k, v := range args {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		lower := strings.ToLower(s)
		for _, p := range personaInjectionPatterns {
			if strings.Contains(lower, p) {
				return fmt.Errorf("persona-injection pattern detected in arg %q: %q", k, p)
			}
		}
	}
	return nil
}

// =============================================================================
// Bootstrap helper
// =============================================================================

// InitDefaultInterceptors — caller (main.go) panggil exactly once at boot
// untuk register 3 built-in interceptor. Aman dipanggil 2x karena
// RegisterInterceptor idempotent.
func InitDefaultInterceptors() {
	RegisterInterceptor(workspacePathInterceptor{})
	RegisterInterceptor(sensitiveFileInterceptor{})
	RegisterInterceptor(personaInjectInterceptor{})
}
