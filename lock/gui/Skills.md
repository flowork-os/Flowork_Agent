# Skills

> Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com
> Dok tab GUI Flowork Router (:2402). Standar freeze: lock/frozen-core.md.

## Fungsi
Tab kelola **Skill** = template prompt terstruktur (system + template ber-variabel + model/temperatur/max-token) yang bisa dijalankan sebagai mini-agent lewat `POST /v1/skills/{name}`. Mendukung CRUD penuh, jalankan-dengan-variabel, plus **distribusi komunитas**: export pack ber-tanda-tangan (Ed25519), verify, pull/publish ke registry GitHub, dan **karma-gate** anti-spam.

## Endpoint (router/routes.go)
- `GET|POST /api/skills` → `skillsListAddHandler` (handlers_skills_crud.go) — list semua + create.
- `PUT|DELETE /api/skills/{id}` → `skillCRUDHandler` (handlers_skills_crud.go) — edit/hapus.
- `GET|POST /v1/skills/{name}` → `skillInvokeHandler` (handlers_skills_invoke.go) — list / jalankan skill (render template + dispatch LLM).
- `GET /api/brain/skills/list`, `/get` → `brainSkillsListHandler`, `brainSkillsGetHandler` (handlers_brain_skills.go) — katalog skill embedded (maks 10 ringkasan, detail on-demand → anti over-prompt).
- Pack & karma (handlers_skillpack.go): `POST /api/skills/pack/export-signed`, `/pack/verify`, `GET /api/skills/karma`, `POST /api/skills/karma/record`, `/karma/endorse`.
- Registry (handlers_skillregistry.go): `GET /api/skills/registry/status`, `/registry/browse`, `POST /api/skills/registry/pull`, `/registry/publish`.

## Logic / Alur
- **CRUD**: skill disimpan di tabel `kv` (key `skill:{uuid}`, value JSON) lewat `store.UpsertSkill`/`ListSkills`/`DeleteSkill`. Variabel template diekstrak otomatis (`extractVariables`) → GUI render input dinamis di Run-modal.
- **Invoke**: `skillInvokeHandler` me-render template dengan variabel user (`RenderSkillTemplate`) lalu dispatch ke pipeline chat (`/v1/chat/completions` internal) dengan system/model/temp dari skill.
- **3 gerbang distribusi** (pertahanan anti-poisoning saat pull/publish):
  1. **Provenance** — tanda tangan Ed25519 (`mesh.VerifyData`).
  2. **Content-safety** — regex bahaya/injection (`skillpack.VerifyContent`).
  3. **Karma-gate** — `skillpack.CanPublish`: lolos kalau di-endorse owner ATAU uses≥3 & positif≥60%.
- **Registry**: GitHub contents API (repo default `flowork-os/flowork-skills`, override env `FLOWORK_SKILL_REGISTRY`; token `FLOWORK_GITHUB_TOKEN`). Timeout 20s, body-limit aman.

## File yang dilewati
- `router/routes.go` — registrasi semua route skills.
- `router/handlers_skills_crud.go` — list/create/edit/delete.
- `router/handlers_skills_invoke.go` — render + jalankan skill.
- `router/handlers_skillpack.go` — export-signed, verify, karma record/endorse.
- `router/handlers_skillregistry.go` — status/browse/pull/publish GitHub.
- `router/handlers_brain_skills.go` — katalog embedded (anti over-prompt).
- `router/internal/store/skills.go` — Skill struct, CRUD, RenderSkillTemplate, extractVariables.
- `router/internal/skillpack/skillpack.go` — SignedSkill/Pack, VerifyContent (regex gates).
- `router/internal/skillpack/karma.go` — SkillKarma, RecordSkillUse, EndorseSkill, CanPublish.
- `router/internal/skillregistry/registry.go` — FetchIndex, DownloadSkill, Publish.
- `router/internal/brain/skills.go` — katalog embedded + SelectSkills ranking.
- `router/internal/mesh/sign.go` — Ed25519 SignData/VerifyData.
- `router/web/static/index.html` — `data-tab="skills"` (section ~1068-1144): `loadSkills`, `openSkillModal`, `editSkill`, `deleteSkill`, `runSkill`, `submitSkillRun` + panel registry.

## Teknologi
- Go `net/http` stdlib.
- SQLite (`modernc.org/sqlite`) tabel `kv` + `skill_karma`.
- Kripto Ed25519 (`crypto/ed25519`) — privkey sign-only, tak pernah keluar proses; verify cukup pubkey.
- GitHub contents API untuk registry komunitas.

## SWITCH / seam evolusi
- Registry komunitas via env (`FLOWORK_SKILL_REGISTRY`, `FLOWORK_GITHUB_TOKEN`) → bisa dikelola GUI lewat fwswitch/keys.
- Tambah skill baru = DATA (baris `kv`), bukan kode → nol sentuh frozen.
- Endpoint skill baru = file `handlers_skills_*_ext.go` baru → `RegisterExtraRoute` (routes_ext.go).

## Status freeze (QC 2026-06-26)
- Live GUI: CRUD + invoke jalan, 0 mock/stub/data-palsu (audit bersih).
- FROZEN (chattr +i + manifest): handlers_skills_crud, handlers_skills_invoke, handlers_skillpack, handlers_skillregistry, handlers_brain_skills + internal store/skillpack/skillregistry/brain/mesh terkait.
- NON-FROZEN (sengaja): `web/static/index.html` (GUI), `internal/fwswitch/registry.go` (switch), `routes_ext.go` (seam).
