// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-07 (pre-freeze audit pass).
// Reason: THE capability gate. Single chokepoint Call() enforces args cap + grant
//   check (GrantAuto auto, GrantOwner/GrantTier must be granted) + provider
//   existence + panic isolation (one provider panic → error Result, kernel +
//   other modules survive). Verified caller id is the transport's job (loopback).
package loket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Provider executes one capability call for a module. It receives the VERIFIED
// caller module id and the raw args, and returns a raw JSON result. A provider
// must treat `module` as the source of truth for isolation — e.g. a storage
// provider operates only on that module's own folder, never another's.
type Provider func(ctx context.Context, module string, args json.RawMessage) (json.RawMessage, error)

// maxArgsBytes caps the size of a single call's arguments — a cheap guard so a
// module cannot hand the kernel an unbounded payload.
const maxArgsBytes = 1 << 20 // 1 MiB

// Kernel is the loket: the single dispatch point. It owns the capability routing
// table and the grant table, and is safe for concurrent use. This struct is the
// whole of the engine's authority — everything else is a provider or a module.
type Kernel struct {
	mu        sync.RWMutex
	providers map[string]Provider     // cap name -> provider
	grants    map[string]map[string]bool // module id -> set of granted cap names
	limiter   *rateLimiter             // nil = no rate limit (default)
}

// NewKernel returns an empty kernel. Builtin providers are registered by the
// caller so the kernel core itself stays dependency-free and trivially testable.
func NewKernel() *Kernel {
	return &Kernel{
		providers: map[string]Provider{},
		grants:    map[string]map[string]bool{},
	}
}

// Register wires a provider for a capability name. Re-registering REPLACES the
// provider — that is how a service is swapped (e.g. pointing llm.complete at a
// local model) without touching the kernel.
func (k *Kernel) Register(cap string, p Provider) error {
	if cap == "" {
		return fmt.Errorf("empty capability name")
	}
	if p == nil {
		return fmt.Errorf("nil provider for %q", cap)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.providers[cap] = p
	return nil
}

// Grant records that a module may call a set of capabilities. Called after the
// owner has approved a module's manifest. Auto-grant caps are allowed implicitly
// at call time, so only owner/tier caps strictly need granting; granting the
// full consumed set is harmless and explicit.
func (k *Kernel) Grant(module string, caps []string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	set := k.grants[module]
	if set == nil {
		set = map[string]bool{}
		k.grants[module] = set
	}
	for _, c := range caps {
		set[c] = true
	}
}

// GrantManifest grants a validated module its consumed capabilities. The
// manifest must already have passed Validate (which enforces the tier rule); the
// owner approval of GrantOwner caps happens in the install flow before this.
func (k *Kernel) GrantManifest(m *Manifest) {
	if m != nil {
		k.Grant(m.ID, m.Consumes)
	}
}

// Revoke drops a module's grants — on uninstall or unload.
func (k *Kernel) Revoke(module string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.grants, module)
}

// Call is THE primitive. A module asks for a capability by name; the kernel
// checks the capability exists, that the (kernel-stamped) module is granted it,
// routes to the provider, and returns the result.
//
// `module` MUST be the verified caller id — never a value taken from an
// untrusted body. Enforcing that is the transport's job (see the loopback
// endpoint), so this method can trust it.
func (k *Kernel) Call(ctx context.Context, module, cap string, args json.RawMessage) Result {
	if len(args) > maxArgsBytes {
		return errResult("args too large (%d bytes, max %d)", len(args), maxArgsBytes)
	}
	if !k.limiter.allow(module) {
		return errResult("rate limit exceeded for module %q", module)
	}
	spec, known := LookupCap(cap)
	if !known {
		// Not a frozen builtin. It may be a service-provided cap that extends the
		// namespace — allow it only if a provider exists AND the module was
		// explicitly granted it (services declare what they provide; consumers
		// declare what they consume, owner-approved).
		if k.hasProvider(cap) && k.isGranted(module, cap) {
			return k.invoke(ctx, module, cap, args)
		}
		return errResult("unknown capability %q", cap)
	}
	if !k.allowed(module, spec) {
		return errResult("capability %q not granted to %q", cap, module)
	}
	if !k.hasProvider(cap) {
		return errResult("capability %q has no provider (service down?)", cap)
	}
	return k.invoke(ctx, module, cap, args)
}

// allowed reports whether `module` may call a known capability.
//   - GrantAuto: always (safe, isolation lives in the provider).
//   - GrantOwner / GrantTier: only if explicitly granted.
func (k *Kernel) allowed(module string, spec CapSpec) bool {
	if spec.Grant == GrantAuto {
		return true
	}
	return k.isGranted(module, spec.Name)
}

func (k *Kernel) hasProvider(cap string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	_, ok := k.providers[cap]
	return ok
}

func (k *Kernel) isGranted(module, cap string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.grants[module][cap]
}

func (k *Kernel) invoke(ctx context.Context, module, cap string, args json.RawMessage) (res Result) {
	k.mu.RLock()
	p := k.providers[cap]
	k.mu.RUnlock()
	if p == nil {
		return errResult("capability %q has no provider", cap)
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	// Fault isolation — the core promise. A buggy provider must NEVER crash the
	// kernel: a panic in one capability becomes an error Result, and the kernel
	// plus every other module keep running. "If A fails, B survives", enforced
	// here at the single chokepoint every call passes through.
	defer func() {
		if r := recover(); r != nil {
			res = errResult("capability %q panicked: %v", cap, r)
		}
	}()
	out, err := p(ctx, module, args)
	if err != nil {
		return errResult("%s", err.Error())
	}
	if len(out) == 0 {
		out = json.RawMessage("{}")
	}
	return Result{OK: true, Result: out}
}

func errResult(format string, a ...any) Result {
	return Result{OK: false, Error: fmt.Sprintf(format, a...)}
}
