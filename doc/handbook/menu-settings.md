# ⚙️ Settings

Your owner-level control panel — global stuff that isn't tied to any one agent (kept in the global
`flowork.db`). Every API key, token and default lives here, in one place, never hardcoded in the code.

The page has these segments:

- **Account** — change your password, or log out.
- **API Keys** — the keys Flowork uses for outside services (stored as secrets, shown masked).
- **Router & Model** — the global default LLM model and router URL.
- **Notifications** — your own Telegram bot, for the system to ping you.
- **YouTube** — connect a YouTube account by OAuth, for the automation that needs it.
- **Guardian** — the integrity guard's status (which files are watched).

---

## API Keys

This is where every external credential goes — your Dev.to key, social cookies, and so on. Nothing is
ever written into the source code; agents read these at boot from here.

**Add a key the easy way:** under the input there's a row of **service chips** (Dev.to, X, LinkedIn,
Telegram…). Tap one and it auto-fills the *exact* variable name for you — you don't have to guess
whether it's `DEVTO_API_KEY` or `DEVTO_KEY`. Then just paste the value and **Save**. A chip turns
**green** once its key is set, so you can see at a glance what's already configured.

**Add a key manually:** type the name yourself (must be `UPPER_SNAKE_CASE`, e.g. `ETHERSCAN_API_KEY`)
in the first box, the value in the second, then **Save**. The name suggestions also appear as you type.

**Edit / Delete:** each saved key shows masked (only the last 4 chars). *Edit* re-fills the name and
clears the value box for a fresh paste; *Delete* removes it.

Some names are **reserved** and refused on purpose — anything that could hijack the process or its child
commands (`PATH`, `LD_*`, `DYLD_*`, `FLOWORK_*`, `HOME`, `GIT_*`, …). That's a safety rail, not a bug.

**How it reaches agents:** when you save a key it's stored in `flowork.db` *and* injected into the
running process immediately (no restart). On the next boot the keys are loaded **before** the agents
start, so an agent always sees them in its environment. A per-platform promo group, for example, reads
its own key via `os.Getenv("DEVTO_API_KEY")`.

---

## Router & Model

The **global default** the agents fall back to when they don't pin their own:

- **Default model** — e.g. `claude-haiku-4-5`. Leave it empty to use the built-in default.
- **Router URL** — e.g. `http://127.0.0.1:2402`. Leave it empty for the built-in local router. Must be a
  localhost address — an external URL is rejected and falls back to the default (a safety rail against
  pointing your traffic somewhere it shouldn't go).

**Per-agent always wins.** If an agent sets its own model (so you can run *one agent, one model*), that
choice is kept. These two values only fill the blank for agents that set nothing. Saving takes effect
live, and is also applied before agents boot.

---

## Notifications

Your own Telegram: paste a bot token + chat id, **Save**, then **Test** (it sends you a test message).
This is the token the whole system uses to ping you — yours, never hardcoded.

---

**Principle:** one place for the owner's keys, defaults and switches, kept apart from each agent's
private settings. If a tutorial or feature ever asks you to paste a key into a file or a per-agent box,
it's out of date — everything global belongs here.
