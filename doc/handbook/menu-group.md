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
