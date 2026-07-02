# Gerbang approval interaktif — F-B v1 (2026-07-02)

## Arsitektur
Mesin approval udah ada dari dulu di frozen core (JANGAN bikin ulang):
- `internal/tools/sandbox_v3.go` (FROZEN): chokepoint `requiresApproval` → enqueue
  `approval_queue` (per tool+args_hash, approved berlaku 1 jam) + sentinel `ErrPendingApprove`.
  Urutan: sensitive-args → ReadOnlyClassifier (exempt) → sensitiveTools → **ExtraGatePolicy**.
- Endpoint (frozen, udah dari dulu): GET `/api/agents/protector/approval/queue?id=<agent>` ·
  POST `.../approve_pending?id=&queue_id=` · POST `.../reject_pending` — **butuh login GUI**
  (bukan loopback-allowlist; cuma owner yang bisa mutusin).

F-B v1 = NGISI colokan yang udah disiapin (semua NON-frozen, deletable):
- `internal/tools/builtins/permission_policy.go` — `tools.ExtraGatePolicy = approvalGatePolicy`
  (mode-aware) + interceptor `approval-mode-agent` (per-agent `approval_mode`='plan' di config
  agent → agent read-only; per-agent CUMA bisa memperketat, ga bisa relaksasi).
- `internal/tools/builtins/cmdsem.go` — git SADAR-SUBCOMMAND: dulu semua `git` dianggap
  read-only → `git push`/`commit` lolos exempt; sekarang cuma subcommand baca (status/log/
  diff/show/rev-parse/…; branch/remote/tag cuma kalau semua arg flag). Test: cmdsem_test.go.
- `internal/fwswitch/registry.go` — switch GUI `FLOWORK_APPROVAL_MODE` (string, default
  `default`, kategori Security / Approval). Live tanpa restart (policy baca os.Getenv host-side).

## Mode (global, switch GUI)
| Mode | Perilaku |
|---|---|
| `default` | Aksi DESTRUKTIF (shell mutasi, termasuk git push/commit) → antrian approval. Read-only + file-tool workspace auto-allow. |
| `acceptEdits` | Alias `default` (edit file workspace emang udah auto-allow di Flowork). |
| `plan` | SEMUA non-read-only → antrian approval. |
| `bypass` | Tanpa gerbang ekstra (protector/caps/ARM tetap aktif). |

`system_power` TIDAK diurus di sini — udah punya gerbang sendiri (cap `exec:power` + ARM +
`FLOWORK_POWER_REQUIRE_APPROVAL`).

## Verifikasi 2026-07-02
E2E bahasa manusia: minta mr-flow `mkdir` → ke-hold `pending owner approval queue_id=1`
(pending pertama itu masih nunggu keputusan owner di GUI), reply edukatif, agent ga stuck.
`git status` (read-only) tetap jalan tanpa approval. Unit: `TestClassifyCommand_GitSubcommand`
+ `TestApprovalGatePolicy_Modes` PASS. Build/vet/test/TestKernelFreeze hijau.

## Sisa F-B (belum, buat penerus)
- Panel GUI antrian pending (endpoint udah ada; tinggal frontend — tab Protector).
- Notif Telegram ke owner pas ada pending baru (biar ga nunggu buka GUI).
- Mode per-agent penuh (relaksasi per-agent SENGAJA ga dibikin — keputusan keamanan).
