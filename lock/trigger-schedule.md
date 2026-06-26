# SCHEDULE + TRIGGER — engine event/jadwal (GUI ⏰ Schedule + ⚡ Trigger)

> Dev: Aola Sahidin. 2026-06-26. Satu engine, dua tab: Schedule = type "time" (cron),
> Trigger = event (webhook/file-watch/dst). Backend sama (`/api/triggers*`). Cara kerja: `os/`.

## JALUR
```
schedule.js / triggers.js (GUI)
  → /api/triggers{,/toggle,/run,/delete,/duplicate,/runs,/types,/hook/{id}}
    (triggers_handler.go = route, NON-frozen seam)
  → triggers.Engine (engine.go): Tick() poll · HandleWebhook() · RunNow() · runAction()
  → type registry (triggers.go: Register/ListTypes) → type_time/webhook/filewatch (Check/OnWebhook)
  → deliver registry (deliver.go: RegisterDeliverer) → telegram/chat
  → storage (floworkdb/triggers.go): trigger_rules + trigger_fired_keys + trigger_runs
SystemAction (mis. "compact-all") di-inject dari main.go (non-frozen).
```

## FROZEN — pipa/jalur (7 file, chattr +i + KERNEL_FREEZE + TestKernelFreeze)
`triggers/engine.go`, `triggers/triggers.go`, `triggers/deliver.go`, `triggers/type_time.go`,
`triggers/type_webhook.go`, `triggers/type_filewatch.go`, `floworkdb/triggers.go`.
(Header white-label, komentar di-strip; token-identik terbukti, build+test PASS.)

## SEAM — nambah TIPE/CHANNEL TANPA buka frozen (plug-and-play, sudah ada)
- **Tipe trigger baru** = file sibling `triggers/type_<x>.go` (NON-frozen) + `func init(){ Register(&xType{}) }`.
  `ListTypes()` auto-detect → form GUI auto-update. engine.go gak disentuh.
- **Channel kirim baru** = `RegisterDeliverer("<kind>", fn)` di file baru. `dispatchDeliver` auto-route.
- **System action baru** = inject di main.go (non-frozen) → `runAction` panggil via target_kind=system.
Pipa (engine/registry/storage + 3 tipe base) beku → tipe/channel baru GAK BISA robohin yang jalan.

## SWITCH
Tidak ada master on/off (mateng: matiin engine = matiin SEMUA cron user = bahaya). Evolusi dijamin
lewat registry `Register`/`RegisterDeliverer` (bukan switch). Enable/disable per-rule = DATA
(`trigger_rules.enabled`, tombol GUI), bukan kode.

## NON-FROZEN (seam, sengaja)
`triggers_handler.go` (route), `feature_ops.go` (route reg), `web/tabs/{schedule,triggers}.js` (GUI),
`type_*.go` BARU (zona tipe), main.go (system action inject).

## VERIFIKASI 2026-06-26
QC live (login :1987): types registry balikin file-watch/time/webhook (config_schema). create
schedule(time) → ok · run manual → run #57 · runs history → 1 · delete → ok (artefak QC dibersihin).
Strip token-identik. Build+TestKernelFreeze PASS. Append → "Operation not permitted".
