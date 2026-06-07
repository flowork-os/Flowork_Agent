// === FROZEN (kernel inti) — DO NOT MODIFY. Kernel FREEZE v1 (2026-06-07). ===
// Owner: Aola Sahidin (Mr.Dev). Bagian microkernel "papan kosong" abadi; checksum
// dipin di KERNEL_FREEZE.md. Ubah = unfreeze eksplisit owner + update manifest.

package loket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Deps are the host-provided dependencies the builtin providers need. The kernel
// core stays decoupled and trivially testable; the host fills these with real
// implementations (folder resolution, the wasm runtime).
type Deps struct {
	// StorePath returns the SQLite path for a module's own store. The host maps
	// it to the module's own folder — that mapping IS the storage isolation.
	StorePath func(module string) (string, error)

	// ModuleDir returns a module's own folder. Used to read its loket.json so the
	// kernel can grant the capabilities it declares it consumes (manifest-driven).
	ModuleDir func(module string) (string, error)

	// IsPrimary reports whether a module is AUTHORITATIVELY a primary-tier agent
	// (engine-fused, owner-blessed). The manifest's own tier field is only a CLAIM;
	// this is the kernel's authoritative answer, so a module cannot self-promote to
	// a tier-gated capability (e.g. the 5M shared corpus) by writing tier:"primary"
	// in its own loket.json. Optional: if nil, the self-declared tier is trusted
	// (dev/test only — production wires this to the owner-controlled allowlist).
	IsPrimary func(module string) bool

	// Send delivers a one-way message to a target module/channel/owner. The
	// kernel has already stamped msg.Source with the verified caller id.
	Send func(ctx context.Context, target string, msg Message) error

	// Invoke delivers a message to a target and returns its reply (RPC). Used by
	// bus.request and bus.broadcast.
	Invoke func(ctx context.Context, target string, msg Message) (json.RawMessage, error)

	// Modules lists the currently loaded modules for discovery (registry.*), so a
	// group/agent can find members or providers of a capability WITHOUT hardcoding
	// ids. Optional: nil → registry.* returns an empty list.
	Modules func() []ModuleInfo

	// NotifyOwner delivers a message to the owner's active channel (Telegram/GUI).
	// Backs bus.send(to:"owner") (§8.E). Optional: nil → that send errors.
	NotifyOwner func(ctx context.Context, text string) error
}

// ModuleInfo is the discovery view of a loaded module (registry.list/providers).
type ModuleInfo struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Provides []string `json:"provides,omitempty"`
}

// RegisterBuiltins wires every SourceKernel provider into k using deps. Service
// providers (llm.complete, brain.shared.*) are registered separately by their
// service modules, so this stays purely the kernel's own builtins.
func RegisterBuiltins(k *Kernel, deps Deps) {
	sp := &storeProviders{deps: deps}
	_ = k.Register("store.kv.get", sp.kvGet)
	_ = k.Register("store.kv.set", sp.kvSet)
	_ = k.Register("store.kv.delete", sp.kvDelete)
	_ = k.Register("store.kv.list", sp.kvList)
	_ = k.Register("store.doc.put", sp.docPut)
	_ = k.Register("store.doc.get", sp.docGet)
	_ = k.Register("store.doc.query", sp.docQuery)
	_ = k.Register("store.doc.delete", sp.docDelete)
	_ = k.Register("store.brain.add", sp.brainAdd)
	_ = k.Register("store.brain.search", sp.brainSearch)

	_ = k.Register("time.now", timeNow)
	_ = k.Register("log", logProvider)
	_ = k.Register("http.fetch", httpFetch)

	// Sandboxed syscalls (fs.* + exec.run) — GrantOwner, path-scoped to the module
	// folder. Registering the provider does NOT grant it; the owner gate stands.
	scp := &syscallProviders{deps: deps}
	_ = k.Register("fs.read", scp.fsRead)
	_ = k.Register("fs.write", scp.fsWrite)
	_ = k.Register("fs.list", scp.fsList)
	_ = k.Register("exec.run", scp.execRun)

	// Discovery — find modules / capability providers without hardcoding ids.
	rp := &registryProviders{deps: deps}
	_ = k.Register("registry.list", rp.list)
	_ = k.Register("registry.providers", rp.providers)

	// Schedule — a module wakes itself later (after a delay or on cron) by having
	// the kernel deliver a {kind:"schedule"} message to its handle.
	sch := &scheduleProviders{deps: deps}
	_ = k.Register("schedule.after", sch.after)
	_ = k.Register("schedule.cron", sch.cron)

	bp := &busProviders{deps: deps}
	_ = k.Register("bus.send", bp.send)
	_ = k.Register("bus.request", bp.request)
	_ = k.Register("bus.broadcast", bp.broadcast)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// ── store providers ──────────────────────────────────────────────────────────

// storeProviders caches one open Store per resolved path. The path encodes the
// module's folder, so caching is per-module and isolation is preserved.
type storeProviders struct {
	deps  Deps
	cache sync.Map // path -> *Store
}

func (sp *storeProviders) open(module string) (*Store, error) {
	if sp.deps.StorePath == nil {
		return nil, fmt.Errorf("store not wired")
	}
	path, err := sp.deps.StorePath(module)
	if err != nil {
		return nil, err
	}
	if v, ok := sp.cache.Load(path); ok {
		return v.(*Store), nil
	}
	st, err := OpenStore(path)
	if err != nil {
		return nil, err
	}
	if actual, loaded := sp.cache.LoadOrStore(path, st); loaded {
		_ = st.Close()
		return actual.(*Store), nil
	}
	return st, nil
}

func (sp *storeProviders) kvGet(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		K string `json:"k"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	v, found, err := st.KVGet(a.K)
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"value": v, "found": found}), nil
}

func (sp *storeProviders) kvSet(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		K string `json:"k"`
		V string `json:"v"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	if err := st.KVSet(a.K, a.V); err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"ok": true}), nil
}

func (sp *storeProviders) kvDelete(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		K string `json:"k"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	if err := st.KVDelete(a.K); err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"ok": true}), nil
}

func (sp *storeProviders) kvList(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Prefix string `json:"prefix"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	keys, err := st.KVList(a.Prefix)
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"keys": keys}), nil
}

func (sp *storeProviders) docPut(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Collection string          `json:"collection"`
		ID         string          `json:"id"`
		Body       json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	if err := st.DocPut(a.Collection, a.ID, a.Body); err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"ok": true}), nil
}

func (sp *storeProviders) docGet(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Collection string `json:"collection"`
		ID         string `json:"id"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	body, found, err := st.DocGet(a.Collection, a.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return mustJSON(map[string]any{"found": false}), nil
	}
	return mustJSON(map[string]any{"found": true, "body": body}), nil
}

func (sp *storeProviders) docQuery(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Collection string `json:"collection"`
		Limit      int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	recs, err := st.DocQuery(a.Collection, a.Limit)
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"records": recs, "count": len(recs)}), nil
}

func (sp *storeProviders) docDelete(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Collection string `json:"collection"`
		ID         string `json:"id"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	if err := st.DocDelete(a.Collection, a.ID); err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"ok": true}), nil
}

func (sp *storeProviders) brainAdd(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Content string `json:"content"`
		Wing    string `json:"wing"`
		Room    string `json:"room"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	id, added, err := st.BrainAdd(a.Content, a.Wing, a.Room)
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"id": id, "added": added}), nil
}

func (sp *storeProviders) brainSearch(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Query string `json:"query"`
		K     int    `json:"k"`
	}
	_ = json.Unmarshal(args, &a)
	st, err := sp.open(module)
	if err != nil {
		return nil, err
	}
	hits, err := st.BrainSearch(a.Query, a.K)
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"hits": hits, "count": len(hits)}), nil
}

// ── sys providers ────────────────────────────────────────────────────────────

func timeNow(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
	return mustJSON(map[string]any{"ts": time.Now().UTC().Format(time.RFC3339)}), nil
}

func logProvider(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Level string `json:"level"`
		Msg   string `json:"msg"`
	}
	_ = json.Unmarshal(args, &a)
	if a.Level == "" {
		a.Level = "info"
	}
	log.Printf("loket[%s] %s: %s", module, a.Level, a.Msg)
	return mustJSON(map[string]any{"ok": true}), nil
}

// ── bus providers ────────────────────────────────────────────────────────────

type busProviders struct{ deps Deps }

func (bp *busProviders) stampMsg(caller, msgType string, payload json.RawMessage) Message {
	if msgType == "" {
		msgType = "message"
	}
	return Message{
		// Source is STAMPED from the verified caller — never read from the body.
		Source:  MsgSource{Kind: "module", ID: caller},
		Type:    msgType,
		Payload: payload,
		Context: MsgContext{TS: time.Now().UTC().Format(time.RFC3339)},
	}
}

func (bp *busProviders) send(ctx context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		To      string          `json:"to"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.To == "" {
		return nil, fmt.Errorf("bus.send: 'to' required")
	}
	// "owner" is a logical address (§8.E): the kernel routes it to the owner's
	// active channel (Telegram/GUI), so a module reaches the human without knowing
	// which channel is live.
	if a.To == "owner" {
		if bp.deps.NotifyOwner == nil {
			return nil, fmt.Errorf("bus.send: owner channel not wired")
		}
		if err := bp.deps.NotifyOwner(ctx, ownerText(a.Payload)); err != nil {
			return nil, err
		}
		return mustJSON(map[string]any{"sent": true, "to": "owner"}), nil
	}
	if bp.deps.Send == nil {
		return nil, fmt.Errorf("bus not wired")
	}
	if err := bp.deps.Send(ctx, a.To, bp.stampMsg(module, a.Type, a.Payload)); err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"sent": true}), nil
}

// ownerText extracts a human-readable string from a bus payload bound for the
// owner: a {"text":"…"} field if present, otherwise the raw payload JSON.
func ownerText(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(payload, &p) == nil && p.Text != "" {
		return p.Text
	}
	return string(payload)
}

func (bp *busProviders) request(ctx context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		To      string          `json:"to"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.To == "" {
		return nil, fmt.Errorf("bus.request: 'to' required")
	}
	if bp.deps.Invoke == nil {
		return nil, fmt.Errorf("bus not wired")
	}
	reply, err := bp.deps.Invoke(ctx, a.To, bp.stampMsg(module, a.Type, a.Payload))
	if err != nil {
		return nil, err
	}
	return mustJSON(map[string]any{"reply": reply}), nil
}

// broadcast fans out to each target and collects replies. Sequential for now —
// a group with many members is correct either way; parallelism is an additive
// optimisation that does not change the contract.
func (bp *busProviders) broadcast(ctx context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		To      []string        `json:"to"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if bp.deps.Invoke == nil {
		return nil, fmt.Errorf("bus not wired")
	}
	type replyEntry struct {
		Target string          `json:"target"`
		Reply  json.RawMessage `json:"reply,omitempty"`
		Error  string          `json:"error,omitempty"`
	}
	replies := make([]replyEntry, 0, len(a.To))
	for _, target := range a.To {
		r, err := bp.deps.Invoke(ctx, target, bp.stampMsg(module, a.Type, a.Payload))
		e := replyEntry{Target: target, Reply: r}
		if err != nil {
			e.Error = err.Error()
		}
		replies = append(replies, e)
	}
	return mustJSON(map[string]any{"replies": replies}), nil
}
