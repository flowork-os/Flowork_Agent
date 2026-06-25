# SCOPED INSTINCT (RI-5) — insting per-peran via X-Agent-ID

> Owner: Aola Sahidin (Mr.Dev). Update: 2026-06-25. Koloni BERLAPIS: tiap semut cuma dapet
> insting domain-nya (coder ≠ bisnis) + baseline universal/tool. Token-efisiensi + anti-noise.

## AKAR yang dicabut
Insting lintas-domain ke-inject ke SIAPA AJA → boros + noise. Router DULU **buta** siapa
pemanggil (`store.APIKey` no agent-id, model `flowork-brain` shared, agent ga kirim identitas).

## RANTAI (end-to-end)
```
agent (selfID) ──X-Agent-ID header──▶ router auth-middleware ──ctx──▶ instinct selector ──filter by Room──▶ inject
```
1. **Agent kirim** `X-Agent-ID: selfID()` HANYA pada call `/v1/chat/completions` — di `fetch()`:
   `agent/agentkit/agentkit.go` (FROZEN — nutup SEMUA worker + agent-template lewat delegasi
   `agentkit.Main()`) + `agents/mr-flow/main.go` (FROZEN, loop sendiri). No-op kalau selfID kosong.
2. **Router parse** header → ctx: `handlers_apikey_auth.go` (sebelum cabang auth → kena keyed+keyless)
   + helper `internal/router/agentctx_ext.go` (`WithAgentID`/`AgentIDFromContext`, sibling authctx LOCKED).
3. **Selector ctx-aware**: `instinctenrich.go` (FROZEN) punya hook ke-2 `instinctSelectHookCtx`
   (`RegisterInstinctSelectorCtx`), call-site PREFER ctx-hook. Logika scoping di
   `internal/router/instinctenrich_ext2.go` (NON-frozen): filter `all` → baseline ∪ domain-peran,
   lalu rank pakai `semanticInstinctSelector` (reuse RI-1 vindex).

## SWITCH (Rule 7 — evolusi tanpa buka freeze)
| ENV | Default | Guna |
|---|---|---|
| `FLOWORK_INSTINCT_SCOPED` | **off** | master switch scoping. off = perilaku PROVEN (semantic, no scope) |
| `FLOWORK_INSTINCT_SCOPE_MAP` | — | role-map RUNTIME (no rebuild): `agent:room1,room2;agent2:room3` |

Role-map default (compiled, `instinctenrich_ext2.go` `roleDomains`): `mr-flow` → semua domain
(generalis, no-op aman). Tambah agent lain di map / ENV buat aktifin scoping-nya.

## FAILS-OPEN (anti-rusak) — di TIAP titik balik ke `semanticInstinctSelector` (perilaku lama):
switch off · agent-id kosong (external / agent belum di-rebuild kirim header) · agent belum di-map ·
hasil filter kosong. **Baseline `instinct_universal` + `instinct_tool` SELALU lolos** → ga ada agent
"buta tool" gara2 scoping.

## VERIFIKASI
- Unit: `go test ./internal/router/ -run TestScoped` (5 PASS: filter, fails-open ×3, baseline).
- Live (Rule-9): `grep "instinct-scope:" /tmp/flowork-watchdog.log` →
  `agent="mr-flow" domains=[coding security crypto bisnis] 284→280 kandidat` + reply koheren (no regression).

## CATATAN
- **Template TAK diubah**: agent-template/worker = `func main(){ agentkit.Main() }` (delegasi) →
  fix agentkit cukup. group/connector-template ga call LLM-router.
- **Agent lama** (deploy belum di-rebuild dgn agentkit baru) ga kirim X-Agent-ID → fails-open (aman).
  Rebuild semut (roadmap #5) bikin mereka ikut scoping.
- Sumber role belum dari manifest (manifest no field role) → role-map = sumber kebenaran (router-side, editable).
