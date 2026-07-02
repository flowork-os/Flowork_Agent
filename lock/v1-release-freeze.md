# v1 Release Freeze — public English seed + stable GUI lock

Scope of the v1.0.1 release audit/freeze. Manifest: `KERNEL_FREEZE.md` (SHA256 + `chattr +i`).

## Public brain seed = English (local stays as-is)
- `router/internal/brain/instinct_seed.json` (296 instincts + 1 persona) and
  `router/internal/brain/doctrine_seed.json` (31 drawers + 14 constitution) are the **embedded
  seed** that populates a FRESH brain on first boot (`seed_instinct.go` / `seed_doctrine.go`, frozen).
- For the public release they are in **English**, personal owner data removed, `AOLA-*` doctrine
  keys → `CORE-*`. Persona name `Mr.Flow` preserved (product identity, not personal data).
- Seeding is **no-op once the brain has data** — an already-populated brain (e.g. the owner's local
  Indonesian brain) is never overwritten. Original Indonesian seeds are backed up privately at
  `flowork-secrets/seed-indo-backup/` (not pushed). See [[brain]], [[FLoworkInstincts]].

## Frozen for v1 (chattr + manifest)
- Seed core: `instinct_seed.json`, `doctrine_seed.json`.
- Stable GUI (html/js): `router/web/static/index.html`, `agent/web/index.html`,
  `agent/web/tabs/settings.js`, `agent/web/tabs/commits.js`.
- `TestKernelFreeze` enforces **.go only**; `.json/.html/.js` entries are `chattr` + manifest
  record. To evolve a frozen GUI file: `chattr -i` → edit → rebuild → re-`sha256sum` → update
  manifest → `chattr +i`.

## Deliberately NOT frozen (rule emas §7 — shared growth surfaces)
- `agent/web/css/style.css` (single shared stylesheet, touched by every UI change).
- `agent/web/i18n/*` (translations grow).
Freezing these would force an unfreeze on every future style/translation edit = growth-surface lock.

## GUI fixes shipped in v1.0.1
- Console Log (router): self-scroll terminal panel + hacker theme (`.term-console`/`.term-scroll`).
- Code Progress (agent): self-scroll `#commits` terminal theme + staggered line-reveal (CSS only;
  `commits.js` untouched). Copy corrected — source is the audit DB, not `git log`.
- Settings (agent): removed dead `renderFinance` zombie (backend `/api/finance/wallet` stays dormant).
