<div align="center">

<img src="img/cover.png" alt="Flowork Agent — self-hosted AI agents that learn from their past and guard their own code" width="100%">

# ⚡ Flowork Agent

### Self-hosted AI agents that live in your machine, **learn from their own past, and guard their own code.**

*Your own personal AI agent host — isolated, plug-and-play, offline-friendly, and watched over by a live security radar. No SaaS. No telemetry. No lock-in.*

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![WASM](https://img.shields.io/badge/runtime-WASM%20(wazero)-654FF0)](https://wazero.io)
[![SQLite](https://img.shields.io/badge/state-SQLite%20WAL-003B57?logo=sqlite&logoColor=white)](https://sqlite.org)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Single Binary](https://img.shields.io/badge/deploy-single%20binary-success)]()
[![Platform](https://img.shields.io/badge/os-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()

**self-hosted AI agent · self-improving agent memory · multi-agent orchestration · Telegram AI bot · MCP server · live code security scanner · 100% local-first**

```bash
git clone https://github.com/flowork-os/Flowork_Agent.git && cd Flowork_Agent && ./start.sh
```
*One command. Single binary. Live on `http://127.0.0.1:1987` — zero external services.*

[Quick Start](#-quick-start) • [Self-Improving Brain](#-self-improving-brain-per-agent--isolated) • [Orchestration](#-multi-agent-orchestration) • [Threat Radar](#️-threat-radar) • [vs OpenClaw / Hermes](#-coming-from-openclaw-or-hermes-agent) • [Architecture](#️-architecture)

</div>

---

## 🧠 What is Flowork Agent?

**Flowork Agent** is a microkernel that hosts **autonomous AI agents** — each one a sandboxed WASM citizen that lives in its own folder with its own persona, tools, schedule, wallet, **brain**, and private SQLite state. Drop a folder in, restart, and the agent is alive. Pull it out, and it's gone — no global config to untangle.

Each agent doesn't just *run* — it **remembers, learns, dreams, and defends its own mind.** And the whole thing is **a single Go binary** you own end-to-end.

> *"Simple is hard. Complicated is easy."* — the doctrine this project is built on.

---

## 🧠 Self-Improving Brain (per-agent · isolated)

> Most agents forget everything the moment the chat ends. Flowork agents **carry their own mind** — and it gets sharper over time.

Every agent has a **local brain in its own `state.db`** (SQLite FTS5) — not a shared cloud, not the router. It keeps working even if the network dies. The brain is a full learning loop:

| | Layer | What it does |
|---|---|---|
| 📓 | **Local memory** | `brain_add` / `brain_search` — the agent stores and recalls its **own experience** via fast FTS5 keyword search. Local-first, no embeddings, no cloud. |
| 📜 | **Constitution** | Sacred, always-injected rules — a **5W1H gate**, identity guard, and anti-hallucination doctrine baked into every prompt. |
| 🔁 | **Mistakes recall** | The agent logs its errors with a hit-count and **recalls past mistakes** before repeating them: *"last time you broke X, the fix was Y."* |
| 💭 | **Dream** | While idle, a rule-based pass consolidates recurring patterns into **eureka** insights — the brain grows richer from the agent's own history. |
| 🛡️ | **Immune system** | An antibody scanner **quarantines prompt-injection / jailbreak / low-confidence** drawers so the brain never gets poisoned. |
| 🔗 | **Federation** | Agents can **promote** their best, vetted knowledge to a shared brain so peers learn from each other — fully optional, works offline. |

**Why "isolated" matters:** the brain travels *with the agent folder*. Clone it, ship it on a USB, run it on another machine — it brings its memory, skills, and dreams. Knowledge is portable and fork-able, Linux-style. One agent ≠ one shared mind — **one agent = its own experience.**

---

## ✨ Features

| | Feature | What it does |
|---|---|---|
| 🤖 | **Plug-and-play agents** | Each agent = a portable folder (WASM + manifest + isolated `state.db`). Install/remove by dropping a folder. True sandbox isolation via WASI. |
| 🧠 | **Self-improving brain** | Per-agent local memory, constitution, mistakes recall, dream consolidation, and an immune system. ([details ↑](#-self-improving-brain-per-agent--isolated)) |
| 🧭 | **Mr.Flow orchestrator** | A built-in router agent: chat it in plain language and it **auto-dispatches a multi-agent task** and delivers the verdict back. ([details ↓](#-multi-agent-orchestration)) |
| 🤝 | **Category Tasks (multi-agent crew)** | Define a crew once — researchers fan out in parallel, a synthesizer fuses their findings into one grounded decision. |
| ⏰ | **Recurring scheduler** | Cron-style automation: *"every day at 9 AM, analyze stock A and send the decision to Telegram."* Set it, forget it. |
| 🔌 | **MCP server** | Expose your agents to **external AI** — Claude Desktop, Claude Code, Cursor — over standard Model Context Protocol. |
| 🖥️ | **Operator agents** | A capability-gated agent can **operate the host** (shutdown / reboot / sleep / lock) on confirmed command — armed, audited, isolated. |
| 🖥️ | **Terminal TUI** | A console cockpit to `list` / `run` / `review` tasks with a live step timeline — same pipeline as the GUI. |
| 🛡️ | **Threat Radar** | A **live background security scanner** with a hacker-style radar UI. Auto-scans your code the moment it changes. ([details ↓](#️-threat-radar)) |
| 💬 | **Telegram-native** | Ship an agent as a Telegram bot in minutes — long-poll updates, persona, multi-turn conversation memory, slash commands. |
| 👛 | **Crypto wallet** | Live multi-chain portfolio (Etherscan + CoinGecko) — ETH/Polygon/Arbitrum, ERC-20 tracking, balance alerts. |
| 💰 | **Finance & budgets** | Per-agent LLM cost ledger, budget guardrails, spend dashboard. |
| 🔐 | **File Protector (HPG)** | Host Protection Gate — immutable rules block destructive ops (`rm -rf /`, secret exfil, metadata pivots) before they run. |
| 🗺️ | **Codemap** | Force-directed dependency graph of your codebase — health scores, import edges, file viewer. |
| ⚙️ | **Owner Settings** | Single-owner auth (bcrypt + session), API keys, personal wallet, notifications — kept **separate** from agents so agents stay portable. |
| 📋 | **Audit everything** | Append-only audit log, decisions journal, karma/reputation, retention policy. |

---

## 🆚 Coming from OpenClaw or Hermes Agent?

Love self-hosted personal AI agents like **[OpenClaw](https://github.com/openclaw/openclaw)** or **[Hermes Agent](https://github.com/nousresearch/hermes-agent)**? Flowork plays in the same yard — agents you own, that live on your machine, talk over Telegram, grow skills, and remember you. Here's where Flowork is **different**:

| | OpenClaw / Hermes | **Flowork Agent** |
|---|---|---|
| **Memory** | session memory / learning loop | per-agent **isolated brain** that travels with the folder (portable, fork-able) |
| **Security** | — | **live Threat Radar** scans your code on every change (secrets, SQLi, SSRF…) — *nobody else ships this* |
| **Anti-hallucination** | prompts / guidance | **constitution + immune system** that quarantines poisoned memory by design |
| **Runtime** | Node / Python | **single Go binary**, WASM-sandboxed agents (WASI isolation), pure-Go SQLite |
| **Footprint** | — | anti-over-prompt budget: 5 core tools, the rest discovered on demand |

> Not a fork, not a clone — a different bet: **isolation, security, and portability first.** If you want an agent that *defends its own mind and your codebase*, you're home.

---

## 🛡️ Threat Radar

> The feature that makes Flowork feel like a hacker movie — and keeps your code honest. **No other agent framework ships a live security scanner.**

- **Background watch:** the moment you (or an AI) edit a `.go`/`.py`/`.js` file, it gets auto-scanned. No CI server, no waiting.
- **Real auditors:** hardcoded secrets (AWS/GitHub/Google/Stripe/Slack/JWT/private-key **by value**), SQL injection, command injection, SSRF, weak crypto, mutex/deadlock, resource leaks, path traversal, and more.
- **Radar UI:** animated sweep, severity blips, live scan log, status `SECURE / NOTED / WARNING / THREAT`.
- **Telegram alerts:** critical/high findings get pushed straight to your phone.

Every fix gets re-scanned automatically — so a patch that opens a new hole gets caught before it ships.

---

## 🧩 Multi-Agent Orchestration

> One message in. A whole crew gets to work. One grounded answer out.

Most "AI agents" are a single model in a loop. Flowork runs a **team**. Talk to **Mr.Flow** — the built-in orchestrator — and it decides whether to answer directly or **assemble a crew**:

```
You (Telegram / GUI / MCP / TUI)
        │  "analyze stock GOTO"
        ▼
   🧭 Mr.Flow  ── routes ──►  📋 Category Task
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                     ▼
        🔎 Fundamentals      📈 Technicals        📰 Sentiment      (crew fans out)
              └────────────────────┼────────────────────┘
                                   ▼
                          🧩 Synthesizer  ──►  ✅ Decision  ──►  📲 back to you
```

- **🧭 Smart routing:** plain chat → Mr.Flow auto-triggers the right task and threads delivery back to you.
- **🤝 Real crews:** each member is an isolated agent with its own tools, persona, and brain. They research independently; a synthesizer fuses everything into one sourced decision.
- **🏗️ Build crews from the GUI:** a visual Task Builder defines categories + crews. Every run has a live step timeline.
- **🔌 Four front doors, one funnel:** drive the same pipeline from **Telegram, the GUI, an MCP client (Claude/Cursor), or the terminal TUI.**
- **📲 No ghosting:** when a task completes, the result is *delivered* — logged at every hop.

---

## 📦 120+ Built-in Tools + Slash Commands

Every agent ships with a deep toolbox — **120+ registered tools** across 10+ domains: memory & brain, file & code, agent ops, audit & journaling, security, wallet & finance, scheduler, skills, federation, and more.

> **Anti-overprompt by design:** only **5 core tools** auto-inject into the prompt — the rest are discovered on demand via `tool_search`. 120+ tools in the shed, never dumped on the LLM at once. Smaller prompts, fewer hallucinations, lower cost.

---

## 🚀 Quick Start

```bash
git clone https://github.com/flowork-os/Flowork_Agent.git
cd Flowork_Agent
./start.sh                      # builds + launches on http://127.0.0.1:1987
```

Open **http://127.0.0.1:1987**, set your owner password, and you land straight on the **Threat Radar**. Single binary, embedded kernel, zero external services required.

> One-click desktop launchers (`flowork-start.desktop` / `flowork-restart.desktop`) included. Background scanner + scheduler + brain-dream cron all start automatically.

**Optional power-ups** (*Settings → API Keys / Notifications*):
- Telegram bot token + chat ID → chat your agent + owner alerts
- `ETHERSCAN_API_KEY` → live wallet balances

---

## 🏗️ Architecture

```
┌─────────────────────────────┐        ┌──────────────────────────┐
│  Flowork Agent  (:1987)     │  HTTP  │  Flowork Router (:2402)  │
│  per-citizen state + brain  │ ─────► │  collective LLM brain    │
│                             │        │                          │
│  • WASM microkernel (wazero)│        │  • multi-provider gateway│
│  • isolated state.db + brain│        │  • shared knowledge corpus│
│  • Threat Radar scanner     │        │  • routing + pricing     │
│  • Telegram daemon          │        │                          │
└─────────────────────────────┘        └──────────────────────────┘
```

- **Portable:** an agent is a folder — brain, skills, and dreams travel with it. Move it, USB it, run it anywhere.
- **Nano-modular:** one file, one job. Drop-in tools register themselves.
- **Multi-OS:** Linux, macOS, Windows. No CGO (pure-Go SQLite).
- **Isolated:** agents can't read each other's state. Router down? Agents keep their full local brain.

---

## 🔗 Pair with Flowork Router (recommended)

For multi-provider LLM routing, a shared knowledge corpus, cost-aware model selection, and mesh sync, run Flowork Agent with its sibling:

### 👉 **[Flowork Router → github.com/flowork-os/flowork_Router](https://github.com/flowork-os/flowork_Router)**

The Agent works **fully standalone** (local brain + your own LLM keys). With the Router it also gets a collective corpus and a smart gateway. **Install both for the full experience.**

---

## 🧩 Tech Stack

`Go 1.25` · `wazero (WASM)` · `modernc SQLite (WAL + FTS5)` · `fsnotify` · `bcrypt` · vanilla-JS GUI · zero heavy deps.

---

## 🏷️ Keywords

self-hosted AI agent · self-improving AI agent · agent memory · local-first AI · personal AI assistant · autonomous agent framework · multi-agent orchestration · agent crew · AI orchestrator · Telegram AI bot · MCP server · Claude Code · LLM agent · recurring agent scheduler · WASM microkernel · Go agent runtime · LLM gateway · code security scanner · secret scanner · SAST · prompt-injection defense · crypto wallet agent · plug-and-play AI · offline AI agent · sandboxed agents · OpenClaw alternative · Hermes agent alternative

---

## 📜 License

MIT © Aola Sahidin (Mr.Dev). Built to outlive its maker — an AI home that keeps running.

<div align="center">

**⭐ Star this repo** if a self-hosted AI agent that *learns from its past and guards your code* sounds like your kind of thing.

[GitHub](https://github.com/flowork-os/Flowork_Agent) • [Router](https://github.com/flowork-os/flowork_Router) • [Telegram](https://t.me/+55oqrk75lc43YWE1)

</div>
