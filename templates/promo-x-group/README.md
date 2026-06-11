# promo-devto — per-platform promo group (Dev.to)

One platform = one group (pasukan-semut). Verified end-to-end 2026-06-11.

## Shape
- **coordinator** (`promo-devto`, this custom wasm) — orchestrates + POSTs to the Dev.to Forem API.
- **promo-devto-writer** — drafts the article (reuses the stock ant wasm + `members/writer.prompt.md`).
- **promo-devto-honesty** — strips overclaim, finalizes (stock ant wasm + `members/honesty.prompt.md`).

## Flow
`handle_message {text:"<source material>"}` → writer drafts → honesty edits → coordinator parses
title/body → POST `https://dev.to/api/articles` (Forem). No API key configured → returns the draft
(never posts). Every member call goes through the router (anti-hallucination antibody + constitution).

## Config (owner, transparent — kv OR workspace file)
Drop in the coordinator's `workspace/`: `devto_api_key` (one line), `publish` ("true"=live, else draft),
`tags` (comma-separated). Members/synthesizer overridable via the Group Colony menu (kv: members, synthesizer).

## Capabilities
`net:fetch:http://127.0.0.1:1987/api/kernel/call` (loket) + `net:fetch:https://dev.to` (Forem) +
state + time. (Broker prefix-match: `net:fetch:https://dev.to` covers all dev.to paths; `net:fetch:*`
does NOT, because the glob stops at `/`.)

## Build
`GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .` (coordinator). Members reuse `templates/ant-template/agent.wasm`.

## Clone for another platform
Copy this folder, swap the POST target (X → twikit cookie; LinkedIn → Voyager cookie; ref code in
`Documents/socmed_ref/`), keep the writer+honesty colony. "1 sosmed 1 group."
