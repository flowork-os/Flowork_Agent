// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: loket transport + caller identity. callerID stamps the VERIFIED caller
//   (constant-time secret compare + idRe shape) — the anti-spoof anchor. Residual
//   (shared secret → per-guest secret) is documented inline; do not relax the check.
package loket

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Service is the loket's HTTP face: the single endpoint where a module makes a
// call(). It wraps the Kernel and resolves the VERIFIED caller id from the
// transport — never from the request body — so a module cannot act as another.
//
// The transport here is loopback HTTP, mirroring how guest modules already reach
// the host today. That is an implementation detail beneath the contract: the
// frozen surface a module sees is still just call(cap, args), and the transport
// could later become a wasm host-function without changing the contract.
type Service struct {
	Kernel         *Kernel
	deps           Deps
	loopbackSecret string
	gui            *guiProviders
	granted        sync.Map // module id -> struct{} once its manifest grants are applied
}

// NewService builds a Service with all builtin providers registered. The
// loopbackSecret is the kernel-injected guest secret used to authenticate the
// caller header; pass "" only in trusted dev/test (then ?module= is honoured).
func NewService(deps Deps, loopbackSecret string) *Service {
	k := NewKernel()
	RegisterBuiltins(k, deps)
	// gui.emit keeps its latest-per-panel snapshot on the Service so GUIHandler can
	// read it back. The store key is the verified caller, so writes stay isolated.
	g := newGUIProviders()
	_ = k.Register("gui.emit", g.emit)
	return &Service{Kernel: k, deps: deps, loopbackSecret: loopbackSecret, gui: g}
}

// GUIHandler serves GET /api/kernel/gui?module=&panel= — the latest live snapshot a
// module pushed via gui.emit. Owner-gated by the auth middleware (the owner's GUI
// reads it); returns {found:false} when nothing has been emitted yet.
func (s *Service) GUIHandler(w http.ResponseWriter, r *http.Request) {
	module := strings.TrimSpace(r.URL.Query().Get("module"))
	if module == "" {
		writeResult(w, Result{OK: false, Error: "module required"})
		return
	}
	entry, ok := s.gui.Latest(module, strings.TrimSpace(r.URL.Query().Get("panel")))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !ok {
		_, _ = w.Write([]byte(`{"found":false}`))
		return
	}
	out, _ := json.Marshal(map[string]any{"found": true, "module": entry.Module, "panel": entry.Panel, "data": entry.Data, "ts": entry.TS})
	_, _ = w.Write(out)
}

// WebhookHandler serves POST /api/kernel/webhook/<module> — the generic inbound
// webhook (§8.H), the push counterpart to a channel that polls. A module OPTS IN by
// setting "webhook_secret" in its OWN store; the endpoint checks it (X-Webhook-Secret
// header or ?secret=) before delivering the raw body to the module's handle as a
// {source.kind:"webhook"} message and returning its reply. No secret set → every
// webhook is refused, so this never becomes an open trigger for a module.
func (s *Service) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeResult(w, Result{OK: false, Error: "POST only"})
		return
	}
	module := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/kernel/webhook/"), "/")
	if !idRe.MatchString(module) {
		writeResult(w, Result{OK: false, Error: "invalid module id"})
		return
	}
	want := s.moduleSecret(module)
	if want == "" {
		writeResult(w, Result{OK: false, Error: "webhook not enabled for this module"})
		return
	}
	got := r.Header.Get("X-Webhook-Secret")
	if got == "" {
		got = r.URL.Query().Get("secret")
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		writeResult(w, Result{OK: false, Error: "bad webhook secret"})
		return
	}
	if s.deps.Invoke == nil {
		writeResult(w, Result{OK: false, Error: "bus not wired"})
		return
	}
	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	reply, err := s.deps.Invoke(ctx, module, Message{
		Source:  MsgSource{Kind: "webhook", ID: "external"},
		Type:    "webhook",
		Payload: json.RawMessage(body),
	})
	if err != nil {
		writeResult(w, Result{OK: false, Error: err.Error()})
		return
	}
	writeResult(w, Result{OK: true, Result: reply})
}

// moduleSecret reads a module's opt-in "webhook_secret" from its OWN store.
func (s *Service) moduleSecret(module string) string {
	if s.deps.StorePath == nil {
		return ""
	}
	path, err := s.deps.StorePath(module)
	if err != nil {
		return ""
	}
	st, err := OpenStore(path)
	if err != nil {
		return ""
	}
	defer st.Close()
	v, _, _ := st.KVGet("webhook_secret")
	return strings.TrimSpace(v)
}

// ensureGranted reads a module's loket.json (in its own folder) ONCE and grants
// the capabilities it declares it consumes. Manifest-driven: the module declares,
// the kernel grants — zero kernel code per module. Auto caps need no grant, so a
// module without a loket.json still works (it just has only the safe caps). The
// manifest's tier rule is enforced by ParseManifest, so an extension can never be
// granted a primary-only capability here.
func (s *Service) ensureGranted(module string) {
	if _, done := s.granted.LoadOrStore(module, struct{}{}); done {
		return
	}
	if s.deps.ModuleDir == nil {
		return
	}
	dir, err := s.deps.ModuleDir(module)
	if err != nil {
		return
	}
	raw, err := os.ReadFile(filepath.Join(dir, "loket.json"))
	if err != nil {
		return // no manifest → auto caps only, which is fine
	}
	m, err := ParseManifest(raw)
	if err != nil || (m.ID != "" && m.ID != module) {
		return // malformed, or claims a different id than the verified caller
	}
	// The manifest's tier is only a CLAIM. The kernel — not the agent's own file —
	// decides who is primary, so a module cannot self-promote to a tier-gated cap
	// (e.g. the 5M shared corpus) by writing tier:"primary" in its loket.json.
	// Override the claim with the authoritative answer, then re-validate at the REAL
	// tier: if its consumed caps aren't allowed there, grant nothing tier-gated.
	if s.deps.IsPrimary != nil {
		real := TierExtension
		if s.deps.IsPrimary(module) {
			real = TierPrimary
		}
		if m.Tier != real {
			m.Tier = real
			if err := m.Validate(); err != nil {
				return
			}
		}
	}
	s.Kernel.GrantManifest(m)
}

// CallHandler serves POST /api/kernel/call with body {cap, args}.
//
// The caller module id comes from the X-Flowork-Caller header, which is
// trustworthy only when accompanied by the loopback secret the kernel injects
// into guests — the same caller-bound identity the existing tool dispatcher
// uses. Without the secret (dev), an explicit ?module= is accepted for testing
// behind the loopback + auth gate.
func (s *Service) CallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeResult(w, Result{OK: false, Error: "POST only"})
		return
	}
	module := s.callerID(r)
	if module == "" {
		writeResult(w, Result{OK: false, Error: "unidentified caller"})
		return
	}
	var body struct {
		Cap  string          `json:"cap"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
		writeResult(w, Result{OK: false, Error: "decode: " + err.Error()})
		return
	}
	if strings.TrimSpace(body.Cap) == "" {
		writeResult(w, Result{OK: false, Error: "cap required"})
		return
	}
	s.ensureGranted(module) // manifest-driven grants, applied once per module
	// 240s (was 120s, owner-approved 2026-06-16): a channel→orchestrator bus.request whose
	// orchestrator delegates to a multi-agent crew on the LOCAL model (slow, ~25 tok/s, many
	// serial LLM calls) needs >120s — else the kernel cuts the call off → channel "loket: no
	// response" (silent fail on Telegram). Cap, not fixed wait: fast calls still return fast.
	ctx, cancel := context.WithTimeout(r.Context(), 240*time.Second)
	defer cancel()
	writeResult(w, s.Kernel.Call(ctx, module, body.Cap, body.Args))
}

func (s *Service) callerID(r *http.Request) string {
	var id string
	switch {
	case s.loopbackSecret != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Flowork-Secret")), []byte(s.loopbackSecret)) == 1:
		id = strings.TrimSpace(r.Header.Get("X-Flowork-Caller"))
	case s.loopbackSecret == "":
		id = strings.TrimSpace(r.URL.Query().Get("module"))
	default:
		return ""
	}
	// A caller id must be a well-formed module id. This rejects malformed/injection
	// ids and keeps the id usable as a storage-folder key without traversal.
	//
	// NOTE (residual, pre-freeze): the loopback secret is shared across guests, so a
	// secret-holder can still claim ANY valid id (see callerID's threat note). The
	// wasm host overwrites X-Flowork-Caller for loopback calls and guests have no raw
	// sockets, which contains it in practice; the complete fix is a PER-GUEST secret
	// so the kernel derives the id from the secret instead of trusting the header.
	if !idRe.MatchString(id) {
		return ""
	}
	return id
}

func writeResult(w http.ResponseWriter, res Result) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(res)
}
