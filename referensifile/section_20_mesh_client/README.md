# Mesh API client (thin HTTP client agent → router)

Section ini ngga ada port langsung — design protocol from scratch.

Agent → Router HTTP API untuk:
- `GET /api/mesh/peers` — list peer host yang router ini ketauin
- `GET /api/mesh/identity` — pubkey host ini
- `POST /api/mesh/broadcast-tool` — agent register tool baru → router broadcast manifest
- `POST /api/mesh/broadcast-mistake` — agent push mistakes promoted → router insert ke mistakes_journal global + broadcast via mesh gossip
- `GET /api/mesh/find-tool?capability=X` — discover tool yang ada di mesh (router cek manifest list)
- `POST /api/mesh/request-knowledge?topic=Y` — minta knowledge pack dari peer (router orchestrate)

Pattern HTTP client dari:
- `agents/mr-flow/main.go::fetch()` (existing host capability)
- `referensifile/section_07_sync_router/bridge.go` (bridge pattern dari brainbridge)

Implementasi pakai package `internal/routerclient/` yang sudah ada (di-extend dengan method mesh API).
