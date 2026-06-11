# 🤖 AI Agent

Where your agents live. Each agent is its own citizen — its own folder, memory, personality, rules,
and list of what it's allowed to do. They share nothing unless you wire them. Disable or delete one
and nothing else notices.

## Install an agent
Drag a `.fwagent.zip` into the drop zone (must contain `manifest.json` + `agent.wasm`, max 64 MiB).
It extracts to `~/.flowork/agents/<id>.fwagent/` and the kernel hot-loads it — no restart. There's
also a **↻ Refresh** button.

## The agent card & its buttons
Each card shows **ID, Kind, Version, State, Caps**. A switch flips it **Active / Disabled**.
- **⚙️ Setting** — the main config popup (below).
- **📊 Diagnostics** — health and info.
- **📚 Educational Errors** — this agent's own "doctrine" store: mistakes it turned into lessons.
- **⧉ Duplicate** — copy this agent.
- **/ Slash** — a quick slash command.
- **⬇ Download** — export it back to a `.fwagent.zip`.
- **🗑 Remove** — delete it (folder + workspace + brain).

## The Setting popup (all isolated to this one agent)
- **Router** — which LLM endpoint it calls + the model name.
- **Prompt** — its system prompt (persona & rules).
- **Tools** — tick what it may use: Telegram, the LLM router, a KV store, the filesystem (its own
  workspace), outbound net fetch.
- **Schedule** — recurring jobs in cron format.
- **Skills** — extra skills it can pick up.

## For developers — make your own agent
An agent is a folder, zipped as `.fwagent.zip`. The easiest start is to copy a template — already a
"loket-native" agent that reaches every capability through `call(cap, args)`:

```
my-agent.fwagent/
├─ manifest.json   the contract
├─ agent.wasm      the compiled agent
├─ main.go         your logic
├─ prompt.md       its persona
└─ doktrin.md      its "lessons" doctrine
```

`manifest.json`:
```json
{
  "id": "my-agent", "version": "1.0.0", "kind": "agent",
  "display_name": "My Agent", "entry": "agent.wasm", "abi_version": 1,
  "memory_max_mb": 16, "timeout_call_ms": 120000,
  "capabilities_required": [
    "net:fetch:http://127.0.0.1:1987/api/kernel/call",
    "state:read", "state:write", "time:read"
  ],
  "exposes_rpc": [
    { "name": "handle_message", "description": "Handle one message.",
      "input_schema": { "type": "object", "properties": {} } }
  ]
}
```

`capabilities_required` is the permission list (it can only do what's declared). `exposes_rpc` is the
functions it offers. Build with plain Go: `GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .`, zip the
folder, drag it in. Tune the rest from the Setting popup.

---

## 🔐 RULE — Where secrets live (ARCHITECTURE, ENFORCED)

**Every secret (token / API key / cookie / webhook secret) lives in ONE place:
Settings → API Keys (the global `secrets` store in `flowork.db`). Nowhere else.**

This is a hard rule, not a preference. It keeps secrets in a single, manageable,
never-committed store and lets the rest of the config travel cleanly.

- ✅ **Centralized in Settings → API Keys:** all connector tokens
  (`TELEGRAM_BOT_TOKEN`, `DISCORD_BOT_TOKEN`, `SLACK_BOT_TOKEN`, `WHATSAPP_TOKEN`,
  `WHATSAPP_WEBHOOK_SECRET`), publishing keys (`DEVTO_API_KEY`, `X_AUTH_TOKEN`,
  `X_CT0`, `FWOS_BOT_TOKEN`, `YT_*`), notify token — everything secret.
- 🚫 **The ONLY exceptions — kept per-agent, NOT in Settings:** the agent's
  **Router endpoint** and **Model**. These are per-agent on purpose (every agent may
  use a different model/router). Their *defaults* live in Settings
  (`router_default_url`, `llm_default_model`); an agent overrides them in its own
  config when it wants something else.
- 🟡 **Non-secret connector config** (`TARGET_AGENT`, allowed-chats / channel lists)
  stays in the connector's own store — it isn't a secret.

### How a secret reaches the agent that needs it
1. You enter it once in **Settings → API Keys** → stored in the global `secrets`.
2. On boot, the kernel injects global secrets into the process env, and
   `buildAgentEnv` forwards a key **only to the agent that declares it** — a
   connector's token reaches that connector alone, never an unrelated agent (so two
   agents can't both poll the same bot).
3. The agent reads it with `os.Getenv("KEY")`. Done.

### Adding a NEW token/secret later — DO NOT touch the frozen kernel
The env-forward path has a **plug-and-play hook** (`kernelhost.EnvForwardKeys`, the
last edit that frozen file will ever need). Register new keys from **non-frozen**
code:
- A **connector** just declares the field as `"type": "secret"` in its `loket.json`
  schema — `connections.GlobalSecretEnvKeys()` derives it automatically, **zero code,
  zero frozen edit**.
- For a non-connector secret, add the key in non-frozen wiring (e.g. extend the
  function wired into `EnvForwardKeys` in `main.go`).
- **Never unlock `internal/kernelhost/kernelhost.go` to add a key.** If you think you
  must, you're doing it wrong — use the hook.

### For AI working on this repo
Do not invent a second secret store. Do not write a token into an agent's
`state.db`, a `manifest.json`, a `loket.json` value, or any committed file. A secret
that isn't in Settings → API Keys is a bug. The only secret-free things that ship are
*names/placeholders* (`PASTE_YOUR_KEY_HERE`).

---

## 🧠 RULE — Persona (prompt) & the two-tier brain (ARCHITECTURE, ENFORCED)

### The prompt lives in the GUI — always
**Every agent's persona/system prompt is the GUI field (Settings → 1. Prompt), backed
by `kv.prompt` in its `state.db`. That is the single source of truth.** The owner
edits the prompt in ONE place, and it reaches the agent's wasm via
`FLOWORK_AGENT_CONFIG.prompt`. This is what makes Flowork repurposable: you built it
to promote Flowork, but another user changes the prompt in the GUI to sell *their*
product — no file editing, no rebuild.

- A `prompt.md` shipped in an agent folder is only a **seed**. On boot,
  `agentmgr.SeedPromptsFromMd` copies it into `kv.prompt` **once, only when the prompt
  is still empty** — after that the GUI is authoritative and the file is never read
  again. Editing the prompt in the GUI is never overwritten by the seed.
- 🚫 **Do NOT hardcode a persona only in `prompt.md` (or bake it into the wasm).** A
  prompt that can't be changed from the GUI is a bug. Read the persona from
  `FLOWORK_AGENT_CONFIG.prompt` (fallback to a built-in default) — the way `mr-flow`
  does — so the GUI override always wins.
- New agent → put its starter persona in `prompt.md`; the seed lands it in the GUI.

### Intelligence is two-tier — and BOTH must keep learning
An agent's intelligence lives in **two brains**, and the design REQUIRES both to grow,
not sit frozen:

1. **Router brain (shared, server-side).** The Flowork Router holds the big collective
   Memory Palace (~5M drawers) and the anti-hallucination antibodies. On *every* LLM
   call it **enriches** the prompt with relevant knowledge + skills + past-mistake
   antibodies, and is reinforced when a hallucination is caught. One brain, many
   bodies — every agent that calls the router gets smarter together.
2. **Per-agent brain (local, in `state.db`).** Each agent owns its own tables:
   `mistakes_local`, `skills`, `educational_errors_cache`, `brain_antibody`,
   `constitution`, `karma_self`, `interactions`. Each agent ships with a seeded
   starter brain (constitution + a baseline antibody/educational-error set).

**The rule:** an agent must be able to **update its own intelligence** — record a new
mistake, learn/curate a skill, cache an educational-error remediation, strengthen an
antibody, log its interactions/decisions — into its local brain, AND feed vetted
knowledge up to the router brain. A wasm that only *reads* a frozen seed and never
writes back is not finished: wire the local-brain writes (mistake capture →
educational-error → antibody; skill authoring; karma) so the colony actually learns.
The router covers the shared layer; the per-agent layer is what makes each body
individually wiser over time.

### For AI working on this repo
- Persona → `kv.prompt` (GUI). Never strand it in a file or the wasm.
- Don't treat the per-agent brain tables as read-only seed data — agents are meant to
  write to them (learn). If you find them empty after real use, that's a gap to close,
  not the intended steady state.
