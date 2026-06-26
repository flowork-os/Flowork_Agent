# CODE PROGRESS — activity/audit log viewer (GUI tab "Code Progress")

> Dev: Aola Sahidin. 2026-06-26. Tab nampilin stream aktivitas agent (tool_call, protector_block,
> scanner_finding, dst) gaya "commit history" — BUKAN git log, tapi event dari `audit_log`.
> Cara kerja: `os/`.

## JALUR
```
commits.js (GUI)
  → GET /api/commits  (feature_compat.go route, NON-frozen)
  → CommitsCompatHandler (agentmgr/legacy_compat_v3.go) — adapt audit_log → tampilan commit
  → store.ListAudit (agentdb/audit.go) → tabel audit_log (append-only)
WRITE: store.AppendAudit (agentdb/audit.go) dipanggil dari sandbox_v3.go (FROZEN), codescan/engine.go
(FROZEN), system_power, dll → tiap aksi/temuan ke-log. /api/audit (agentmgr/audit.go) = inject/query.
```

## FROZEN — pipa/jalur (3 file, chattr +i + KERNEL_FREEZE + TestKernelFreeze)
`agentdb/audit.go` (store: ensureAuditSchema/AppendAudit/ListAudit + watchdog), `agentmgr/audit.go`
(AuditLogHandler), `agentmgr/legacy_compat_v3.go` (CommitsCompatHandler + compat shim).
(Header white-label, komentar di-strip; token-identik terbukti, build+test PASS.)

## SEAM — nambah jenis event TANPA buka frozen (inheren, DATA-driven)
`audit_log.event_type` = kolom STRING bebas. Nambah jenis event = panggil `AppendAudit(event_type,...)`
dengan string baru dari MANA AJA (gak ada enum/whitelist di kode). GUI `commits.js` render generic
(event_type + detail) → jenis baru otomatis tampil. **Zero edit frozen.** Append-only → integritas
audit kejaga (evolusi nambah row, gak bisa nimpa/hapus histori yang jalan).

## SWITCH
Tidak ada — viewer pasif read-only. Evolusi = data (event_type string), bukan toggle. Append-only by
design → "robohin yang jalan" mustahil dari sisi data.

## NON-FROZEN (seam, sengaja)
`web/tabs/commits.js` (GUI render generic), `feature_compat.go` (route reg), pemanggil `AppendAudit`
(sandbox/codescan/dll — frozen di subsistem masing2). Compat endpoint BARU = file baru.

## VERIFIKASI 2026-06-26
QC live (login :1987): `/api/commits` balikin stream real (scanner_finding auto-scan + tool_call,
hash 7-char, author, subject). Strip token-identik. Build+TestKernelFreeze PASS. Append → "Operation
not permitted".
