# 🔌 Connections

One roof for everything coming in and out. **Two kinds**: channels (how people talk to your agents)
and MCP servers (tools from the outside world your agents can use).

## 1) Channels — human ↔ agent
The doors people use to reach an agent: Telegram, Discord, Slack, WhatsApp, CLI, and so on.
- **Install** — drop a `.fwpack` (a `kind:channel` pack). It validates, extracts to its own folder,
  hot-loads — no restart.
- Each connector card: **Enable / Disable**, **Config** (set its token + settings — fields come from
  the connector's manifest, secrets masked), **🗑 Uninstall**. *Native* connectors only have Config.

## 2) MCP — external tool servers for your agents
MCP (Model Context Protocol) is a standard way to plug external tools into AI. Flowork speaks it
**both ways**.

**Using outside tools (MCP client).** Paste the same `mcpServers` JSON you'd use in any MCP-compatible
app:
```json
{
  "github": {
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "env": { "GITHUB_TOKEN": "..." }
  }
}
```
Hit **Install + enable**. Flowork starts that server; each of its tools registers in the engine as
`mcp_<connector>_<tool>`. Enabled connectors auto-start again on every boot. Each MCP card shows its
tools + Enable/Disable + Uninstall.

An agent then calls a tool through the loket: `call(tool.run, {name: "mcp_<id>_<tool>", args})`. For a
loket-native agent or group, that needs two grants in its own manifest/loket: `tool.run` in `consumes`,
and the connector capability `mcp:<id>` in `capabilities_required`. (Example: the **repo-reviewer**
group enriches each review with live repo metadata via `mcp_web_github_repo`.)

**Exposing your agents (MCP server).** Flowork can also *be* the MCP server, so an outside AI app or
IDE can drive your agents. It speaks MCP over stdio and exposes `chat` (talk to an agent — same path
as Telegram or the CLI), `task_list`, `task_run`, and `task_result`. Point your external client at
the `flowork-mcp` command; the target agent comes from `FLOWORK_MCP_AGENT`.

## For developers — make a connector
A channel connector is a `kind:channel` `.fwpack`. Its manifest declares the config fields it needs
(like a bot token), and the Config panel renders them. Start from `templates/connector-template/`,
fill in the relay logic (a dumb pipe: message in → an agent → reply out), build, and drop the
`.fwpack` in. For MCP you usually build nothing — just paste a server's config (client) or point an external
client at Flowork's MCP server. If you want a **sovereign, no-npm** tool server, a tiny Go stdio MCP
server is easy too — see `cmd/flowork-mcp-web` (exposes `github_repo`); install it by pointing the
connector's `command` at the built binary.
