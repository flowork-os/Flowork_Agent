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
