# Scam Detector

A hacker-terminal crypto-token scam/rug scanner — **anti-hallucination by design**.

```
GUI (ui/index.html)  →  core.py  →  ① GROUND: check_token (GoPlus/RugCheck, FACTUAL)
                                    ②  scam-detector GROUP debates the factual data
                                    ③  synthesizer → ONE verdict (SCAM / HIGH-RISK / CAUTION / LOOKS-OK / UNKNOWN)
```

The intelligence lives in an agent **colony**, not in this app. The app is a thin
orchestrator: it pulls *factual* on-chain data first (deterministic truth, no LLM
guessing), then hands ONLY that data to the colony so the verdict is grounded —
"kecerdasan dulu, anti-halu".

## Parts

| Part | Where | Role |
|---|---|---|
| App | `agent/apps/scam-detector/` | GUI + `core.py` orchestrator (this folder) |
| `scam-detector` | `agent/agents/scam-detector.fwagent/` | GROUP coordinator (fan-out + debate + synthesize) |
| `scam-detector-onchain` | `agent/agents/…onchain.fwagent/` | on-chain flag analyst — also holds `net:fetch:*` so it can run `check_token` |
| `scam-detector-pattern` | `agent/agents/…pattern.fwagent/` | known-scam-pattern analyst |
| `scam-detector-verdict` | `agent/agents/…verdict.fwagent/` | synthesizer → final verdict |

The member/coordinator `agent.wasm` is the **shared** ant/group template
(`templates/ant-template/agent.wasm`, `templates/group-template/agent.wasm`) — it is
git-ignored per repo convention; the persona/config files (`manifest.json`,
`prompt.md`, `doktrin.md`, `group.json`) are committed.

## Provisioning the colony (fresh install)

The app degrades gracefully (returns the factual report with a note) if the colony
is absent, but for the full grounded verdict provision it once:

1. For each member folder under `agent/agents/scam-detector-*.fwagent/`, drop in the
   shared template wasm: `cp templates/ant-template/agent.wasm <member>/agent.wasm`
   (and `templates/group-template/agent.wasm` → `scam-detector.fwagent/agent.wasm`).
   The hot-reload watcher loads them; each member seeds its persona from `prompt.md`.
2. Register the group + debate mode (owner session):
   ```
   POST /api/groups/create  {"id":"scam-detector","display_name":"Scam Detector"}
   POST /api/groups/config?id=scam-detector
        {"members":["scam-detector-onchain","scam-detector-pattern"],
         "synthesizer":"scam-detector-verdict","mode":"debate","debate_rounds":"2"}
   ```
   (`group.json` already carries the roster for `SeedFromJSON`; `mode=debate` is set here.)

## Config (env, no hardcode)

`core.py` reads: `FLOWORK_AGENT_URL` (default `http://127.0.0.1:1987`),
`SCAM_GROUND_AGENT` (default `scam-detector-onchain`), `SCAM_GROUP`
(default `scam-detector`), `SCAM_HTTP_TIMEOUT`.

## Tested (2026-06-17)

- USDC (eth, real `check_token`) → CAUTION/20 (proxy) → colony verdict **LOW RISK**.
- Honeypot report (99% sell-tax, hidden owner) → colony verdict **SCAM CONFIRMED**.
- Full GUI path `/api/apps/op` → grounded verdict. UI renders (green-on-black terminal).
