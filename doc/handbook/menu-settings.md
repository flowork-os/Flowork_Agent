# ⚙️ Settings

Your owner-level control panel — global stuff that isn't tied to any one agent (kept in the global
`flowork.db`).
- **Account** — change your password, or log out.
- **API Keys** — add, edit, or delete the keys Flowork uses (stored as secrets, shown masked).
- **Notifications** — your own Telegram: paste a bot token + chat id, *Save*, and *Test* (sends you a
  test message). This is the token the whole system uses to ping you — yours, never hardcoded.
- **YouTube** — connect a YouTube account by OAuth, for the automation that needs it.

**Principle:** one place for the owner's keys and switches, kept apart from each agent's private
settings.
