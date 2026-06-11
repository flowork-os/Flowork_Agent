# Architecture & Technology

## The stack

- **Go 1.25**, compiled to a single static binary — no cgo, no Docker, no runtime to install.
  Linux / macOS / Windows.
- **A tiny "forever" core (microkernel)** — written once and never touched again. Everything else
  clips onto one fixed contract (ABI); if a plugin breaks, you fix that one folder and nothing else
  cares.
- **Each agent in its own box (WASM)** — agents run as sandboxed WebAssembly via *wazero*, limited to
  the capabilities they're granted.
- **Memory in SQLite** — fast full-text search (FTS5); every agent gets its own private brain file.
- **MCP, both ways** — use outside MCP tools, and let outside apps use your agents.
- **It guards itself** — the core watches for tampering and drops into safe-mode if anything's off.
- **The web UI is embedded** in the binary — no separate site to host.

## The loket (one door)

Everything flows through **one counter (the "loket")**. A module can do nothing alone — to think,
remember, run a tool, or send a message, it asks the kernel for a capability by name: `call(cap, args)`.
The kernel checks the grant, routes to a provider, enforces the sandbox, returns the result.

## How automation flows — Schedule → Group → members

This is the spine of how recurring work actually runs in Flowork, and it is
deliberate. Read it once and the whole system clicks:

```
  Schedule / Trigger  →  activates a GROUP  →  the group's COORDINATOR
   (a clock or event)      (a colony)            hands each sub-task to its
                                                 MEMBER agents (the roster)
                                                 over the loket bus  →  result
```

**A schedule does not drive a loose agent for real work — it activates a GROUP.**
A group is a *coordinator* agent (custom WASM) plus a **roster** of small, sharp
member agents (a writer, a tagger, a reviewer …). When the schedule fires, the
coordinator's `handle_message` receives the trigger's prompt (e.g. `/auto`,
`/review`), splits the job, and dispatches each piece to a member via
`call(bus.request, …)`. Each member does ONE small thing; the coordinator collects
their answers (the synthesizer can fuse them) and acts — post, decide, publish.
You see and edit that roster in the **Group** menu; a group is marked by `group=1`
in its own loket store, with its `members` list beside it.

The mental model is always three layers: **clock → colony → ants.** You grow the
system by adding a member to a roster, never by bloating one do-everything agent.

Colonies that run exactly this way today:

- **promo-devto / promo-x** — a scheduled group writes one grounded post and
  publishes it (Dev.to article; X thread linking the article).
- **repo-reviewer** — a scheduled group reviews a trending GitHub repo (grounded in
  its README, enriched with live metadata via an MCP tool) across channels.
- **investment / thinking** — scheduled multi-agent analysis groups.

## Project map

```
Flowork_Agent/
├─ main.go + *.go ........ the microkernel + HTTP handlers (the "forever" core)
├─ start.sh / stop.sh / restart.sh ... run / stop / rebuild scripts
├─ internal/ ............. core Go packages
│   ├─ kernel/ , kernelhost/ ... the WASM microkernel + capability broker
│   ├─ loket/ ............ the one "counter" every module calls through (the ABI)
│   ├─ guardian/ , protector/ .. self-protection (tamper → safe-mode)
│   ├─ floworkdb/ , agentdb/ ... databases (global + each agent's private brain)
│   ├─ tools/ ............ the built-in tools
│   ├─ scanner/ , scanapi/ ... the security scanner
│   ├─ triggers/ , scheduler/ , taskflow/ ... automation
│   ├─ connections/ , mcpclient/ , mcphub/ ... channels + MCP
│   └─ groupsapi/ , slashcmd/ , settingsapi/ , routerclient/ ...
├─ web/ ................. the control panel UI (index.html, js/, tabs/, i18n/)
├─ apps/ ............... sandboxed apps (flowalpha, notepad)
├─ agents/ ............. installed agents (each a .fwagent folder)
├─ templates/ ......... starting points (agent, group, connector, lens …)
├─ cmd/ ............... extra entry points (CLI, TUI, MCP, chat)
└─ sdk/ , doc/ , scripts/ , seeds/ , voice-backend/
```

The golden rule: the **core** (top-level `*.go` + `internal/kernel`) is written once and left alone.
Everything you'd ever add is its own folder that snaps on.
