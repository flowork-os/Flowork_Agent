# ⏰ Schedule

Jobs that run on a clock. Hit **＋ New**, pick an *agent* or a *group*, write what it should do, and
give it a cron time (`min hr dom mon dow`) — e.g. `0 7 * * *` for 7am daily. It can repeat or fire
once.

Each scheduled job is a card with **Enable/Disable**, **▷ Run** (run now), **✎ Edit**,
**▸ History** (past runs), and **🗑 Delete**.

> ⏱️ **Cron is read in the machine's LOCAL time** (the Schedule/Trigger engine uses
> the server clock). On a WIB box, `0 20 * * *` = 20:00 WIB.

## What a schedule actually fires

This is the firm rule, and it matters: **a schedule activates a GROUP — it does not
poke a loose agent.** When the job fires, the group's **coordinator** receives your
prompt (e.g. `/auto`, `/review`) and **hands each part of the work to its member
agents (its roster)** over the loket bus, then assembles the result and acts. So the
flow is always:

> **Schedule → activates the GROUP → the coordinator distributes tasks to the member
> agents in the roster → result.**

Point a schedule at a group, give it a prompt and a cron time, and the whole colony
runs unattended. (You *can* target a single agent for a simple ping, but real work is
a group.) See [Group](menu-group.md) and [Architecture](architecture.md) for the full
clock → colony → ants picture.

**Principle:** recurring work without you babysitting it. Schedule and [Trigger](menu-trigger.md) are
two faces of the same engine — Schedule is the time-based half. Both fire the same
way: target → group → members.
