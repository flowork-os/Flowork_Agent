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
