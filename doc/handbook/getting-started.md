# Getting Started

## What is Flowork?

Flowork is a **microkernel** — a tiny, eternal core written once and never edited — that hosts
**autonomous AI agents** as sandboxed WebAssembly citizens. Each agent lives in its own folder with
its own persona, rules, tools, schedule, and a **brain** in a private database. Everything else —
agents, tools, scanners, channels, MCP servers, apps — is a plug-and-play module that snaps onto one
frozen contract. Break one, fix one folder; nothing else is touched.

Most AI forgets you the moment you close the tab. A Flowork agent is something you **own**: it lives
on your machine, carries its own memory, learns from its own mistakes, and keeps working offline.
Clone the folder to a USB stick and its whole mind comes with it.

## Why Flowork

- **It's yours.** Local-first, self-hosted. No SaaS, no telemetry, no lock-in. Works fully offline.
- **It snaps together.** Drop in a `.fwpack` and it hot-loads — no kernel edits, no rebuilds.
- **It gets smarter.** Agents learn from what they got wrong, treating mistakes as lessons (see
  [The Mind](the-mind.md)).
- **It watches its own back.** A real [security scanner](menu-threat-radar.md) guards the code your
  agents run — something no other agent framework ships.
- **MCP both ways.** Use external MCP tools, and expose your agents to MCP clients.

A little history: Flowork has been rebuilt **12 times** in about a year and a half — a convergent
search for the right shape of one idea. It began as a browser-based, Python, n8n-style canvas where
*you* did the wiring; it became an agent OS where the **agents** do the orchestrating and you just
own them. Four things never changed: it's always an OS, your data is always yours, everything plugs
in cleanly, and privacy comes first.

## Install

No Docker, no accounts, no cloud. One command:

```
git clone https://github.com/flowork-os/Flowork_Agent.git
cd Flowork_Agent
./start.sh
```

`start.sh` builds the binary on first run (needs **Go 1.25+**) and serves the control panel at
`http://127.0.0.1:1987`. On first launch, create your **owner account** on the login screen — that's
you, the boss.

- Works on **Linux, macOS, and Windows**.
- Stop with `./stop.sh`, restart with `./restart.sh`.
- Everything runs on your machine and talks to nothing outside unless you tell it to.

Next: open the panel and look around. Each menu is documented in this handbook — see the
[index](README.md).
