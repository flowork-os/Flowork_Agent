<div align="center">

# ⚡ Flowork Agent

### Self-hosted AI agents that actually live in your machine — isolated, plug-and-play, and watched over by a real-time security radar.

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![WASM](https://img.shields.io/badge/runtime-WASM%20(wazero)-654FF0)](https://wazero.io)
[![SQLite](https://img.shields.io/badge/state-SQLite%20WAL-003B57?logo=sqlite&logoColor=white)](https://sqlite.org)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Single Binary](https://img.shields.io/badge/deploy-single%20binary-success)]()
[![Platform](https://img.shields.io/badge/os-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()

**AI agent framework · autonomous Telegram agent · live code security scanner · LLM gateway · 100% self-hosted**

[Quick Start](#-quick-start) • [Features](#-features) • [Threat Radar](#-threat-radar) • [Architecture](#-architecture) • [Router (recommended)](#-pair-with-flowork-router-recommended)

</div>

---

## 🧠 What is Flowork Agent?

**Flowork Agent** is a microkernel that hosts **autonomous AI agents (warga)** — each one a sandboxed WASM citizen that lives in its own folder with its own persona, tools, schedule, wallet, and private SQLite state. Drop a folder in, restart, and the agent is alive. Pull it out, and it's gone — no global config to untangle.

It's **single-binary, self-hosted, and offline-friendly.** No SaaS, no telemetry, no vendor lock-in. Your agents, your machine, your data.

> *"Simple is hard. Complicated is easy."* — the doctrine this project is built on.

---

## ✨ Features

| | Feature | What it does |
|---|---|---|
| 🤖 | **Plug-and-play agents** | Each agent = a portable folder (WASM + manifest + isolated `state.db`). Install/remove by dropping a folder. True sandbox isolation via WASI. |
| 🛡️ | **Threat Radar** | A **live background security scanner** with a hacker-style radar UI. Auto-scans your code the moment it changes. ([details ↓](#-threat-radar)) |
| 💬 | **Telegram-native** | Ship an agent as a Telegram bot in minutes — long-poll updates, persona, multi-turn **conversation memory**, slash commands. |
| 👛 | **Crypto wallet** | Live multi-chain portfolio (Etherscan + CoinGecko) — ETH/Polygon/Arbitrum, ERC-20 tracking, balance alerts. |
| 💰 | **Finance & budgets** | Per-agent LLM cost ledger, budget guardrails with warnings, spend dashboard. |
| 🔐 | **File Protector (HPG)** | Host Protection Gate — 28 immutable rules block destructive ops (`rm -rf /`, secret exfil, metadata pivots) before they run. |
| 🗺️ | **Codemap** | Force-directed dependency graph of your codebase — health scores, import edges, file viewer. |
| 📚 | **Doktrin Edukasi** | A catalog of "error → guidance" doctrines so agents follow a playbook instead of getting stuck or hallucinating. |
| 🛠️ | **Tool Caps** | Per-agent capability + tool subscription management — grant exactly what each agent may touch. |
| ⚙️ | **Owner Settings** | Global owner console: single-owner auth (bcrypt + session), API keys, personal wallet, notification routing. Kept **separate** from agents so agents stay portable. |
| 📋 | **Audit everything** | Append-only audit log, decisions journal, karma/reputation, retention policy. |

---

## 🛡️ Threat Radar

> The feature that makes Flowork feel like a hacker movie — and keeps your code honest.

- **Background watch:** the moment you (or an AI) edit a `.go`/`.py`/`.js` file, it gets auto-scanned. No CI server, no waiting.
- **Real auditors:** hardcoded secrets (AWS/GitHub/Google/Stripe/Slack/JWT/private-key **by value**, not just by name), SQL injection, command injection, SSRF, weak crypto, mutex/deadlock, resource leaks, path traversal, and more.
- **Radar UI:** animated sweep, severity blips (critical → core, low → rim), live scan log, status `SECURE / NOTED / WARNING / THREAT`.
- **Telegram alerts:** critical/high findings get pushed straight to your phone.
- **Lock-aware:** ships with a "what's not locked yet" auditor so you always know which files still need a security pass.

Every fix gets re-scanned automatically — so a patch that opens a new hole gets caught before it ships.

---

## 🚀 Quick Start

```bash
git clone https://github.com/flowork-os/Flowork_Agent.git
cd Flowork_Agent
./start.sh                      # builds + launches on http://127.0.0.1:1987
```

Open **http://127.0.0.1:1987**, set your owner password, and you land straight on the **Threat Radar**. That's it — single binary, embedded kernel, zero external services required.

> One-click desktop launchers (`flowork-start.desktop` / `flowork-restart.desktop`) are included. Background scanner + scheduler + watchdog all start automatically.

**Optional power-ups** (set in *Settings → API Keys / Notifications*):
- `ETHERSCAN_API_KEY` → live wallet balances
- Telegram bot token + chat ID → owner alerts

---

## 🏗️ Architecture

```
┌─────────────────────────────┐        ┌──────────────────────────┐
│  Flowork Agent  (:1987)     │  HTTP  │  Flowork Router (:2402)  │
│  per-citizen state + UI     │ ─────► │  collective LLM brain    │
│                             │        │                          │
│  • WASM microkernel (wazero)│        │  • multi-provider gateway│
│  • isolated state.db/agent  │        │  • shared knowledge mesh │
│  • Threat Radar scanner     │        │  • routing + pricing     │
│  • Telegram daemon          │        │                          │
└─────────────────────────────┘        └──────────────────────────┘
```

- **Portable:** an agent is a folder. Move it, ship it on a USB, run it anywhere.
- **Nano-modular:** one file, one job. Drop-in tools register themselves.
- **Multi-OS:** Linux, macOS, Windows. No CGO (pure-Go SQLite).
- **Isolated:** agents can't read each other's state. The owner console is a separate global store.

---

## 🔗 Pair with Flowork Router (recommended)

For the **best performance** — multi-provider LLM routing, shared knowledge brain, cost-aware model selection, and mesh sync — run Flowork Agent together with its sibling:

### 👉 **[Flowork Router → github.com/flowork-os/flowork_Router](https://github.com/flowork-os/flowork_Router)**

The Agent works standalone, but with the Router it gets a collective brain and a smart LLM gateway. **Install both for the full experience.**

---

## 🧩 Tech Stack

`Go 1.25` · `wazero (WASM)` · `modernc SQLite (WAL)` · `fsnotify` · `bcrypt` · vanilla-JS GUI · zero heavy deps.

---

## 🏷️ Keywords

self-hosted AI agent · autonomous agent framework · AI agent platform · Telegram AI bot · WASM microkernel · Go agent runtime · LLM gateway · code security scanner · secret scanner · SAST · crypto wallet agent · plug-and-play AI · offline AI agent · sandboxed agents

---

## 📜 License

MIT © Aola Sahidin (Mr.Dev). Built to outlive its maker — an AI home that keeps running.

<div align="center">

**⭐ Star this repo** if a self-hosted, security-watched AI agent home sounds like your kind of thing.

[GitHub](https://github.com/flowork-os/Flowork_Agent) • [Router](https://github.com/flowork-os/flowork_Router) • [Telegram](https://t.me/+55oqrk75lc43YWE1)

</div>
