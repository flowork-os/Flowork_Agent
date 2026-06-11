# 👥 Group

A team of agents that tackle one task together — a colony of ants, each doing one small job, then a
synthesizer brings the pieces together. Many small, focused agents beat one big do-everything agent.

## Create a group
Type an **ID** and a **Name**, hit **+ Create**. The new group appears as a card.

## The group card
- **Name & ID** — the name is editable.
- **Members** — chips of available agents; tick the team. An agent can only be on one group at a time.
- **Synthesizer** — one agent that combines everyone's answers (or "none").
- **Task** — what the team should do.
- **Save** · **🗑 Delete**.

## How it runs
A group is a **coordinator** agent plus a **roster** of small member agents. The coordinator receives
the task, **fans it out to each member over the internal "loket bus"** (`call(bus.request, …)`),
collects their answers, and the synthesizer stitches them into one result. Members are the agents you
tick in the card — they live *under* the group (an agent belongs to one group at a time).

**A schedule or [Trigger](menu-trigger.md) activates the group, not a loose agent.** The firm flow is:

> **Schedule → activates the GROUP → the coordinator distributes tasks to the member agents in the
> roster → result.**

So you never wire a clock straight to a worker agent — you point it at the colony, and the coordinator
hands out the work. Clock → colony → ants. (See [Architecture](architecture.md) for the same picture at
the system level, and [Schedule](menu-schedule.md) to put a group on a timer.)

## For developers
The simplest group is **no code** — create, tick members, pick a synthesizer, write the task, Save.
Under the hood a group is an agent built from a template: a coordinator whose `handle_message` routes
the task to its members via `call(cap, args)`. For custom orchestration (phases, roles), start from
`templates/group-template/` or a richer example like `templates/investment-group/`, edit `main.go`,
and build it like any agent (`GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .`). Members are ordinary
agents — a great group is really about small, sharp specialists wired together.

---

## 🔗 RULE — Cross-group data: shared workspace vs the bus

When a SEQUENCE crosses groups (e.g. scan a repo → write an article about it →
share it), one step needs the previous step's output. There are two channels.

### `workspace/` IS the shared workspace (SharedDir)
The project's `workspace/` folder is the cross-agent **SharedDir**. Layout:
`workspace/<agent-id>/{tools,job,document,media,cache,log}` per agent, plus
`workspace/_global/` for material shared across every agent. It's git-ignored
(runtime data, may hold scraped content) — only the convention ships.

But an agent only sees `/shared` (this folder) mounted **when it effectively holds
the `fs:shared` capability**, which is a DANGEROUS cap gated by
`FLOWORK_PRIVILEGED_AGENTS`. So file-sharing through `workspace/` is **opt-in and
deliberate**, never blanket: grant `fs:shared` to a specific agent that genuinely
needs cross-agent files, and add it to `FLOWORK_PRIVILEGED_AGENTS`. Do NOT hand
`fs:shared` to ordinary content groups — it gives them read/write over every other
agent's files.

### Prefer the loket BUS for sequenced hand-offs
For chaining steps, the **loket bus is the right channel** — capability-gated,
isolated, no privileged FS grant. A downstream group asks an upstream one for its
latest output, exactly how `promo-x` pulls the article it shares:

```
askMember("promo-devto", "/latest")   // → {ok,topic,title,url}
```

Same pattern wires a full pipeline: `promo-devto` asks `repo-reviewer` for the repo
it just scanned and writes an article about THAT repo; `promo-x` asks `promo-devto`
for the article and shares it to X / Telegram. The data flows group-to-group with
zero cross-agent filesystem access.

**Rule:** sequence hand-offs go over the **bus** by default. Reach for `fs:shared`
+ `workspace/_global` only for genuinely file-shaped, large, or binary artifacts an
agent must drop for others — and grant the cap deliberately, per agent.

---

## 🔌 RULE — How a group is wired (plug-and-play lifecycle)

A group is plug-and-play: create it once and it slots into everything automatically.
None of the below is hardcoded — it follows the group list the kernel keeps in sync.

### A new group auto-appears as a Telegram slash command
The host auto-discovers every group (kv `group=1`) and writes the `id|command|desc`
list into the orchestrator's (`mr-flow-next`) kv `"groups"` on **boot, create, config,
and delete** (`groupsapi/orchestrator.go → SyncToOrchestrator`). The telegram-channel
re-reads that list every few polls and pushes it to Telegram's `setMyCommands`
(deduped). So **create a group → its `/command` shows up in the bot within minutes;
delete it → it disappears.** The command is the id with dashes turned into
underscores (Telegram rejects dashes): `promo-devto` → `/promo_devto`. Zero config,
zero code.

### A group runs from THREE places — all the same logic
- **Schedule** (`#schedule` / `trigger_rules` type=time): `host.InvokeAgentMessage` →
  the coordinator's `handle_message`.
- **Trigger** (event-driven): same `InvokeAgentMessage` path.
- **Mr.Flow**: type `/promo_devto <task>` in chat, or let the model pick the
  `ask_group` tool — the loket **bus** → the coordinator's `handle`.

All three converge on the SAME coordinator dispatch (`case "handle_message", "handle"`)
→ fan-out to members → synthesizer. The group behaves identically whoever triggers it.

### Group ON/OFF (one switch for the whole colony)
The Groups tab has an **ON/OFF** switch per group (`POST /api/groups/toggle`). OFF
disables the coordinator **and every member** — each leaves the runtime, so no agent
in the group can receive a command (the coordinator can't fan out, members can't be
reached over the bus). ON brings them all back.

### Reset to default (restore a deleted group)
Deleted a group by accident? The Groups tab's **↻ Restore defaults**
(`POST /api/groups/reset`) copies every bundled group/agent that's missing back from
the repo (`agents/<id>.fwagent` — definition only, never workspace/secrets), restores
its roster from the committed `group.json`, and re-syncs. Existing agents are never
touched. The factory groups always come back.

### For AI working on this repo
The group list is **derived, never hardcoded** — a group is any module with kv
`group=1`; the slash menu, `ask_group`, schedules and Mr.Flow all read the same
auto-synced list. Don't hand-maintain kv `"groups"`. Don't bake a group's roster into
code — it lives in the loket store + the committed `group.json` mirror.
