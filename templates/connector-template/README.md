# Connector Template

Copy-paste starting point for a new **Connection** (Telegram / Discord / Slack /
email-API / …). A connector is a **dumb pipe**: it carries a line of text to an
agent over the loket bus and sends the reply back. All thinking lives in the agent.

## Make your own in 5 steps

1. **Copy** this folder → `agents/<your-id>.fwagent/` (or zip it into a `.fwpack`).
2. **Edit `loket.json`** → set `id` + `name`. Keep `kind:"channel"`, `entry:"handle"`,
   and `consumes:["bus.request"]`. A connector must NOT request `fs.*` / `exec.run` /
   `http.fetch` (the install gerbang refuses those — it polls via the wasm host fetch,
   not a loket cap).
3. **Fill the three `TODO(connector)` spots** in `main.go`:
   - `config()` — rename the env vars to your platform (token etc.).
   - `pollPlatform()` — long-poll/fetch new messages from your platform's HTTP API.
   - `sendToPlatform()` — POST a reply back to your platform's HTTP API.
   The dumb-pipe core (`hostFetch`, `loketCall`, `forwardToAgent`, `handle`) is generic.
4. **Build** the wasm (one binary, runs on Windows/macOS/Linux):
   ```
   GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
   ```
5. **Install** via the **Connections** tab → set the token → enable. It hot-loads.

## Rules of the road (why it's shaped like this)

- **HTTP only.** Use your platform's official HTTP API; never a raw socket. That keeps
  the connector pure-wasm and portable to every OS with no per-OS build.
- **Polling by default.** Works behind NAT on any desktop. (Webhook is for servers.)
- **Self-contained.** Everything the connector needs is in this folder, token included
  (`connector.json`, written by Connections). Delete the folder = the connector and its
  credential are gone. A crash here can't reach other connectors or the kernel.
- **Idle without a token.** `boot()` exits cleanly if no token is set, so a freshly
  installed connector sits harmlessly until the owner configures it.

## Test without a live platform

`handle`/`handle_message` is the testable core. Invoke it over the loopback kernel RPC
with a synthetic message — the connector→bus→agent path runs for real, no platform needed:

```
curl -s localhost:1987/api/kernel/rpc -d \
  '{"plugin":"<your-id>","function":"handle_message","args":{"text":"halo"}}'
```

You should get back `{"reply":"…","target":"…"}` from the agent.
