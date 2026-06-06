// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06
// Reason: FROZEN ABI v1 of the "papan kosong" microkernel — the eternal
//   capability vocabulary + wire types. Changing or removing an entry breaks
//   every module built against it, forever. EXTEND only: append a new versioned
//   capability to Catalog and bump ABIVersion; NEVER edit/remove an existing one.
//
// Package loket is the Flowork microkernel's single service counter ("loket").
//
// The whole engine exposes exactly ONE primitive to every module:
//
//	call(cap, args) -> result
//
// A module can do nothing on its own. To read its own storage, talk to another
// module, reach the LLM, send a Telegram message, or run a command, it must ask
// the loket for a CAPABILITY by name. The kernel decides whether the module is
// granted that capability, routes the request to the provider, enforces the
// sandbox, and returns the result.
//
// Because there is exactly one entry point and a FROZEN vocabulary of
// capabilities, the kernel never needs new functions — new features are
// routing-table DATA, not kernel code. That is what makes the kernel eternal:
// written once, never edited. The contract may only ever GROW (a new capability
// VERSION added beside the old one); an existing capability is never removed or
// renamed, so a module built today keeps working forever.
//
// This file defines the frozen CONTRACT: the capability vocabulary and the wire
// types. The dispatcher, providers, manifest loader and transport live in
// sibling files. The package is ADDITIVE and does not touch the existing kernel:
// modules reach the loket over loopback, and legacy agents keep their old paths
// until they are migrated.
package loket

import "encoding/json"

// ABIVersion is the contract version this build implements. A module declares
// the version it targets in its manifest. The kernel keeps supporting every past
// version forever: to evolve we ADD a v2 capability beside v1 and bump this — we
// never break v1.
const ABIVersion = "1"

// Grant is how strictly a capability is gated before a module may call it.
type Grant int

const (
	// GrantAuto: low risk, granted to any module automatically (own storage,
	// logging, time, sending a message).
	GrantAuto Grant = iota
	// GrantOwner: high risk, the owner must approve it at install time
	// (filesystem outside the module's own folder, exec, raw network, desktop,
	// wallet).
	GrantOwner
	// GrantTier: gated by the module's tier. The 5M shared brain corpus is
	// primary-only; an extension module is refused even if the owner approves.
	GrantTier
)

func (g Grant) String() string {
	switch g {
	case GrantAuto:
		return "auto"
	case GrantOwner:
		return "owner"
	case GrantTier:
		return "tier"
	default:
		return "unknown"
	}
}

// Source is who actually provides a capability.
type Source int

const (
	// SourceKernel: an irreducible builtin the kernel must provide itself
	// (storage isolation, the bus, sandboxed syscalls). Cannot be a module.
	SourceKernel Source = iota
	// SourceService: provided by a privileged-but-swappable service module
	// (the LLM router, the shared-brain service). Swapping the LLM service to a
	// local model never touches the kernel — this is sovereignty.
	SourceService
)

// CapSpec describes one capability in the frozen vocabulary.
type CapSpec struct {
	Name   string // dotted name, e.g. "store.kv.set" — FROZEN, never renamed.
	Grant  Grant
	Source Source
	Desc   string
}

// Catalog is the FROZEN capability vocabulary for ABI v1.
//
// Rules for eternity:
//   - NEVER rename or remove an entry — a module built against it would break.
//   - To evolve, APPEND a new entry (optionally a ".v2" name) and bump ABIVersion.
//   - Each entry's argument/result schema is defined alongside its provider and
//     is equally frozen.
var Catalog = []CapSpec{
	// A. STORE — the module's own folder (isolated DB). Auto-granted.
	{"store.kv.get", GrantAuto, SourceKernel, "read a key from own config store"},
	{"store.kv.set", GrantAuto, SourceKernel, "write a key to own config store"},
	{"store.kv.delete", GrantAuto, SourceKernel, "delete a key from own config store"},
	{"store.kv.list", GrantAuto, SourceKernel, "list keys in own config store"},
	{"store.doc.put", GrantAuto, SourceKernel, "upsert a record in an own collection"},
	{"store.doc.get", GrantAuto, SourceKernel, "read a record from an own collection"},
	{"store.doc.query", GrantAuto, SourceKernel, "query records from an own collection"},
	{"store.doc.delete", GrantAuto, SourceKernel, "delete a record from an own collection"},
	{"store.brain.add", GrantAuto, SourceKernel, "store a knowledge drawer in own local brain (FTS, dedup)"},
	{"store.brain.search", GrantAuto, SourceKernel, "search own local brain"},

	// B. BUS — talk to other modules / channels / owner. Kernel routes; never direct.
	{"bus.send", GrantAuto, SourceKernel, "send a one-way message to a target"},
	{"bus.request", GrantAuto, SourceKernel, "send a message and await a reply (RPC)"},
	{"bus.broadcast", GrantAuto, SourceKernel, "fan out to many targets and collect replies (groups)"},

	// C. SYSCALL (sandboxed) — high risk, owner-approved.
	{"fs.read", GrantOwner, SourceKernel, "read a file within granted scope"},
	{"fs.write", GrantOwner, SourceKernel, "write a file within granted scope"},
	{"fs.list", GrantOwner, SourceKernel, "list a directory within granted scope"},
	{"http.fetch", GrantOwner, SourceKernel, "make an outbound HTTP request"},
	{"exec.run", GrantOwner, SourceKernel, "run a sandboxed command (e.g. a scanner)"},

	// D. TIME / SCHEDULE / LOG — auto.
	{"time.now", GrantAuto, SourceKernel, "current timestamp"},
	{"schedule.after", GrantAuto, SourceKernel, "schedule a future handle() call after a delay"},
	{"schedule.cron", GrantAuto, SourceKernel, "schedule a recurring handle() call"},
	{"log", GrantAuto, SourceKernel, "append a structured line to own audit log"},

	// E. DISCOVERY — find other modules / providers without hardcoding ids.
	{"registry.list", GrantAuto, SourceKernel, "list modules matching a filter"},
	{"registry.providers", GrantAuto, SourceKernel, "list modules that provide a capability"},

	// F. SHARED SERVICES — provided by swappable service modules; gated.
	{"llm.complete", GrantAuto, SourceService, "ask the LLM (router service; swap to a local model freely)"},
	{"brain.shared.search", GrantTier, SourceService, "search the 5M shared corpus (PRIMARY tier only)"},
	{"brain.shared.promote", GrantTier, SourceService, "contribute a drawer to the 5M shared corpus (PRIMARY only)"},

	// G. GUI — push live data to the module's own declared panel.
	{"gui.emit", GrantAuto, SourceKernel, "push live data to own GUI panel"},

	// H. TOOLS (bridge) — invoke the engine's tool surface by NAME. tool.specs
	// lists the OpenAI function schemas to offer the LLM; tool.run executes one and
	// returns its result. Routing is DATA: today a name resolves to the in-engine
	// tool registry; tomorrow the same name can resolve to a folder module (§D)
	// without touching the kernel. The per-tool sandbox/consent gates still apply
	// beneath tool.run, so this is a second lock, not a bypass. Added 2026-06-06
	// (owner-approved); APPEND only, ABIVersion stays "1" — a pure addition never
	// rejects a module built against the old vocabulary.
	{"tool.specs", GrantAuto, SourceService, "list the tool schemas exposed to the LLM"},
	{"tool.run", GrantOwner, SourceService, "execute a registered tool by name (engine sandbox + consent apply)"},
	{"slash.run", GrantOwner, SourceService, "dispatch a slash command (/cmd …) via the engine slash registry"},
}

// capIndex lets the dispatcher resolve a capability by name in O(1).
var capIndex = func() map[string]CapSpec {
	m := make(map[string]CapSpec, len(Catalog))
	for _, c := range Catalog {
		m[c.Name] = c
	}
	return m
}()

// LookupCap returns the spec for a capability name. ok is false when the name is
// not in the frozen vocabulary — an unknown capability is always refused.
func LookupCap(name string) (CapSpec, bool) {
	c, ok := capIndex[name]
	return c, ok
}

// Request is one call() from a module to the loket.
type Request struct {
	// Module is the caller id. It is STAMPED by the kernel from the verified
	// transport identity, never trusted from the request body — a module cannot
	// act as another module.
	Module string          `json:"module"`
	Cap    string          `json:"cap"`
	Args   json.RawMessage `json:"args"`
}

// Result is the loket's reply to a call().
type Result struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// MsgSource identifies where an inbound message came from. Kind and ID are
// stamped by the kernel and are unforgeable.
type MsgSource struct {
	Kind string `json:"kind"` // "channel" | "module" | "schedule" | "owner"
	ID   string `json:"id"`
}

// Message is what the kernel passes to a module's handle(msg) entry.
type Message struct {
	Source  MsgSource       `json:"source"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Context MsgContext      `json:"context"`
}

// MsgContext carries kernel-provided context for one handle() call.
type MsgContext struct {
	SelfID string `json:"self_id"`
	TS     string `json:"ts"`
}
