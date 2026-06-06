# 🛠️ Building for Flowork — Agents, Scanners, Connectors & the CLI

> Everything in Flowork is **a folder + a manifest**. You extend the system by **dropping a module in**, never by editing the kernel. This guide shows how to build each kind, using the real templates in [`templates/`](templates/).
>
> **The golden rule:** a module talks to the engine through **one primitive** — `call(cap, args)`. Copy a template → fill it in → build the wasm → drop it in. That's the whole loop.

**Contents**
1. [Quick mental model](#1-quick-mental-model)
2. [Build an Agent (the "ant")](#2-build-an-agent-the-ant)
3. [Build a Scanner pack](#3-build-a-scanner-pack)
4. [Build a Connector (channel)](#4-build-a-connector-channel)
5. [Add an MCP connector (no build)](#5-add-an-mcp-connector-no-build)
6. [Install & use the CLI](#6-install--use-the-cli)
7. [Packaging as a `.fwpack`](#7-packaging-as-a-fwpack)

---

## 1. Quick mental model

| Kind | Lives as | Manifest | Built from |
|---|---|---|---|
| **Agent** | `agents/<id>.fwagent/` | `manifest.json` | `templates/ant-template/` |
| **Group** (ant colony) | `agents/<id>.fwagent/` | `manifest.json` | `templates/group-template/` |
| **Connector** (channel) | `agents/<id>.fwagent/` | `loket.json` | `templates/connector-template/` |
| **Scanner** | `<nuclei-templates>/flowork-pack-<id>/` | `plugin.json` (`kind:scanner`) | a folder of `checks/*.yaml` |
| **MCP connector** | `~/.flowork/connectors/mcp/<id>/` | `config.json` (`mcpServers` JSON) | — (an existing MCP server) |

- **Agents** are wasm modules loaded by the runtime. They reach capabilities through the loket at `http://127.0.0.1:1987/api/kernel/call`.
- **Connectors** are dumb pipes — same wasm shape, but `kind:channel`, just forwarding messages.
- **Scanners** are *data* (Nuclei YAML checks), not code.
- Everything installs through one uniform gate: `POST /api/plugins/install` (a `.fwpack` zip) — the kernel reads `kind` and routes it. Drop a `.fwpack` into the dropbox folder and it auto-installs.

> **Build target for all wasm modules:** `GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .`

---

## 2. Build an Agent (the "ant")

An agent is a tiny specialist that does **one job** and reaches every capability through `call(cap, args)`. Small prompt, one job — so even a small/local model can run it.

### 2.1 Anatomy — `templates/ant-template/`

```
my-agent.fwagent/
├── manifest.json     # the agent's identity card (kernel reads this)
├── main.go           # the wasm logic — talks to the loket
├── prompt.md         # the persona  (who the agent is)        → /workspace/prompt.md
├── doktrin.md        # the doctrine (sacred anti-hallucination rules)
├── go.mod
└── agent.wasm        # the built binary (you compile this)
```

`prompt.md` and `doktrin.md` are **plain, editable files that travel with the folder.** The persona and the sacred rules are transparent — no hidden system prompt.

### 2.2 `manifest.json` — the standard

```jsonc
{
  "id": "my-agent",                 // unique, lowercase
  "version": "1.0.0",
  "kind": "agent",                  // agent | group | channel | scanner | tool
  "display_name": "My Agent",
  "description": "Does one specific job.",
  "abi_version": 1,
  "author": "@you",
  "license": "MIT",
  "entry": "agent.wasm",            // the wasm the kernel runs
  "memory_max_mb": 16,
  "timeout_call_ms": 120000,
  "capabilities_required": [        // what it's allowed to reach
    "net:fetch:http://127.0.0.1:1987/api/kernel/call",
    "state:read", "state:write", "time:read"
  ],
  "exposes_rpc": [                  // functions the kernel can call
    {
      "name": "handle_message",
      "description": "Handle one message: remember it, recall, answer.",
      "input_schema": { "type": "object", "properties": {} }
    }
  ]
}
```

> The deep contract (tiers, brain, constitution, karma) is documented in [`doc/standar_ai_agent.md`](doc/standar_ai_agent.md). Read it before changing the kernel/agent contract.

### 2.3 How the code reaches the engine

The kernel runs the module with `os.Args = [name, function, argsJSON]` and reads its stdout. Inside, the agent does its real work by calling the **loket**:

```go
//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

// call(cap, args) → ask the kernel for a capability by name
func call(cap string, args any) (json.RawMessage, error) {
    body, _ := json.Marshal(map[string]any{"cap": cap, "args": args})
    // POST to http://127.0.0.1:1987/api/kernel/call via host_net_fetch …
    // the host stamps THIS agent's verified id + the loopback secret (un-forgeable)
}
```

Common capabilities an ant uses:
- `llm.complete` — ask the LLM (swap to a local model freely).
- `store.brain.add` / `store.brain.search` — its **own** FTS5 memory.
- `store.kv.*` / `store.doc.*` — its own config / structured records.
- `bus.request` / `bus.broadcast` — talk to other agents / a GROUP.
- `tool.run` / `tool.specs` — use the engine's tools (including MCP tools).

The persona + doctrine are read from `/workspace/prompt.md` and `/workspace/doktrin.md` and prepended to the LLM messages. The **constitution** (sacred rules) is auto-injected by the engine every turn.

### 2.4 Build & install

```bash
cp -r templates/ant-template agents/my-agent.fwagent
cd agents/my-agent.fwagent
# edit manifest.json (id, name), prompt.md (persona), doktrin.md (rules), main.go (logic)
GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
```

Then either **restart** (the kernel auto-discovers folders in `agents/`) or package it as a `.fwpack` ([§7](#7-packaging-as-a-fwpack)). The agent appears in the **AI Agent** tab. *Tip: the same wasm becomes a different ant just by changing the manifest + persona.*

---

## 3. Build a Scanner pack

A scanner pack is **data, not code** — a bundle of [Nuclei](https://github.com/projectdiscovery/nuclei) detection templates that join the security **Arsenal**.

### 3.1 Anatomy

```
my-scanner.fwpack (a zip):
├── plugin.json
└── checks/
    ├── exposed-env-file.yaml
    └── debug-endpoint.yaml
```

`plugin.json`:
```json
{
  "id": "my-scanner",
  "kind": "scanner",
  "scanner": { "name": "My Checks", "description": "Custom detections." }
}
```

Each `checks/*.yaml` is a standard Nuclei template, e.g.:
```yaml
id: exposed-env-file
info:
  name: Exposed .env file
  severity: high
http:
  - method: GET
    path: ["{{BaseURL}}/.env"]
    matchers:
      - type: word
        words: ["DB_PASSWORD", "SECRET_KEY"]
```

### 3.2 Install

```bash
# package the folder into my-scanner.fwpack (zip), then:
curl -F file=@my-scanner.fwpack http://127.0.0.1:1987/api/plugins/install
# or the scanner-specific route:
curl -F file=@my-scanner.fwpack http://127.0.0.1:1987/api/scanner/packs/install
```

**The gate:** every check is run through `nuclei -validate` on the way in — invalid templates are rejected, so a broken pack can't poison the arsenal. Checks are inert at install (Nuclei runs without `-code`). Manage from the **Threat Radar → Arsenal** modal (install / uninstall / list). Uninstall removes the pack folder.

> **Rule:** Flowork's scanner is **detection, not weaponization.** Write checks that *find and prove* a hole — never ones that exploit or damage. Run only against targets in your owner-controlled allow-list.

---

## 4. Build a Connector (channel)

A connector is a **dumb pipe**: it carries a message from an external surface (Telegram, Discord, …) to an agent over the bus and relays the reply. It owns no brains.

### 4.1 Anatomy — `templates/connector-template/`

```
my-channel.fwagent/
├── loket.json        # kind:channel manifest + config schema
├── main.go           # the dumb-pipe core + 3 TODO spots
├── go.mod
└── agent.wasm        # built
```

`loket.json`:
```jsonc
{
  "id": "my-channel",
  "kind": "channel",
  "name": "My Channel",
  "version": "0.1.0",
  "abi_version": "1",
  "entry": "handle",
  "tier": "extension",
  "consumes": ["bus.request"],     // a connector needs ONLY the bus
  "config": [                       // the GUI renders these fields automatically
    { "key": "MY_TOKEN",     "label": "Bot Token",    "type": "secret" },
    { "key": "TARGET_AGENT", "label": "Target agent", "type": "text", "default": "mr-flow-next" }
  ]
}
```

> **Security:** a connector must consume **only** `bus.*` (and uses the wasm `host_net_fetch` import to poll its platform). The install gate **refuses** any high-risk capability (`fs.*`, `exec.run`, `http.fetch`) — a connector has no business with those.

### 4.2 Fill the three `TODO(connector)` spots

The generic dumb-pipe core (`hostFetch`, `loketCall`, `forwardToAgent`, `handle`) is provided. You only fill:

1. **`config()`** — the env keys your platform needs (e.g. `MY_TOKEN`).
2. **`pollPlatform()`** — long-poll/fetch new messages from your platform's **HTTP API**.
3. **`sendToPlatform()`** — POST a reply back via the platform's **HTTP API**.

> **Why HTTP only:** wasm modules have one network primitive (`host_net_fetch` = HTTP). Use your platform's official HTTP API (Telegram Bot API, Gmail API, Discord interactions, WhatsApp Cloud API). That keeps the connector pure-wasm → it runs on Windows/macOS/Linux with **no per-OS binary**. Default to **polling** (works behind NAT).

### 4.3 Build & install

```bash
cp -r templates/connector-template agents/my-channel.fwagent
cd agents/my-channel.fwagent
# edit loket.json (id, name, config keys), fill the 3 TODOs in main.go
GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
```

Install via the **Connections** tab (or a `.fwpack`). Then set the token in **Connections → your channel → Config** — it's stored in the connector's **own folder** (masked in the UI), never duplicated elsewhere. Idle until a token is set, so it loads safely. *The agent it feeds defaults to `mr-flow-next`.*

---

## 5. Add an MCP connector (no build)

The **second kind** of connector: hook up an external **MCP server** (GitHub, filesystem, …) so its tools become available to your agents. **No code, no build** — just paste config.

1. Open **Connections → MCP → Add an MCP server**.
2. Paste the same `mcpServers` JSON you'd use in Claude Desktop:
   ```json
   { "github": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"],
                 "env": { "GITHUB_TOKEN": "ghp_…" } } }
   ```
3. **Install + enable.** Flowork spawns the server, lists its tools, and registers each as `mcp_github_<tool>` in the engine registry.

Now **every agent** can use them via `tool_search` → `tool.run` (default-on). Don't want it on a specific agent? Open that agent's **Setting → 🔗 MCP servers** and **uncheck** it. (See [`doc/mcp.json.example`](doc/mcp.json.example).)

---

## 6. Install & use the CLI

`flowork-connect` is a **host-side CLI connector** — chat your agents from the terminal. It also doubles as the project's automated test harness (it drives the same message path as Telegram).

### 6.1 Install

```bash
go build -o bin/flowork-connect ./cmd/flowork-connect
```

### 6.2 Use

```bash
# one-shot
./bin/flowork-connect "hello, who are you?"

# piped — one message per line
echo "what's 2+2?" | ./bin/flowork-connect

# interactive REPL
./bin/flowork-connect

# pick a target agent / base URL, and save it as the default
./bin/flowork-connect --agent mr-flow-next --base http://127.0.0.1:1987 --save

# raw JSON output (for scripting/tests)
./bin/flowork-connect --json "ping"
```

| Flag | Meaning |
|---|---|
| `--agent <id>` | which agent to talk to (default `mr-flow-next`) |
| `--base <url>` | the Flowork base URL (default `http://127.0.0.1:1987`) |
| `--json` | print the raw JSON reply |
| `--debug` | ask the agent for debug detail |
| `--save` | persist `--agent`/`--base` to the CLI's own config, then continue |

**Self-managed config:** the CLI keeps its settings in its own folder — `~/.flowork/connectors/cli/config.json` (override with `FLOWORK_CONNECT_CONFIG`). Multi-OS (paths resolved at runtime).

> Under the hood it POSTs to the loopback `POST /api/kernel/rpc` (`handle_message`) — the same entry the live Telegram connector drives, so a CLI reply is identical to a Telegram reply.

---

## 7. Packaging as a `.fwpack`

A `.fwpack` is just a **zip** the uniform gate (`POST /api/plugins/install`) reads. Layout by kind:

```
# agent / channel pack
plugin.json                      # { "id": "...", "kind": "agent"|"channel" }
agents/<id>/manifest.json (or loket.json)
agents/<id>/agent.wasm

# scanner pack
plugin.json                      # { "id": "...", "kind": "scanner", "scanner": {...} }
checks/*.yaml
```

```bash
zip -r my-agent.fwpack plugin.json agents/
curl -F file=@my-agent.fwpack http://127.0.0.1:1987/api/plugins/install
# …or drop it into the dropbox folder for auto-install.
```

Install **validates** the manifest, asks you to **consent** to any dangerous capability, extracts atomically, and **hot-loads** it (no restart). Uninstall removes the folder, clean. **Break one → fix one folder. Nothing else is touched.**

---

> Questions or a module to share? PRs for new agents, connectors, scanners, and tools are welcome — the kernel never needs to change. See [README](README.md) and [`doc/standar_ai_agent.md`](doc/standar_ai_agent.md).
