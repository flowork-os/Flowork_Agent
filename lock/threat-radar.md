# THREAT RADAR — security scanner (GUI ⌖ Threat Radar)

> Dev: Aola Sahidin. 2026-06-26. Subsistem scan keamanan kode: auto-scan-on-change (fsnotify) +
> manual scan + tool eksternal (trivy/nmap/nuclei), hasil di tab "⌖ Threat Radar" (:1987).
> Cara kerja mesin: lihat `os/`. Prinsip: pipa beku, deteksi (auditor) plug-and-play.

## JALUR (GUI → handler → engine → auditor → storage)
```
scanner.js (GUI)
  → /api/agents/scanner/{scan,runs,findings,auditors}  (agentmgr/scanner.go)
  → /api/scanner/{run,allowlist,registry,registry/toggle} (scanapi/*)
  → scanner.Run (scanner/runner.go) → dispatch Auditors map (scanner/auditors.go)
  → tool eksternal (scanner/tool_immune.go: trivy/nmap/nuclei)
  → simpan run+findings (agentdb/scanner.go); gate exec/target (floworkdb/scan_allowlist.go)
codescan/engine.go = auto-scan-on-filechange (fsnotify debounce) + baseline scan boot.
scanner_scan.go = tool buat agent manggil scan sendiri.
```

## FROZEN — pipa/jalur (11 file, chattr +i + KERNEL_FREEZE + TestKernelFreeze)
`scanner/runner.go`, `scanner/auditors.go`, `scanner/tool_immune.go`, `agentmgr/scanner.go`,
`agentdb/scanner.go`, `scanapi/scanner_allowlist.go`, `scanapi/scanner_registry.go`,
`scanapi/bodyscan.go`, `floworkdb/scan_allowlist.go`, `codescan/engine.go`,
`tools/builtins/scanner_scan.go`. (Header white-label, komentar di-strip; token-identik terbukti.)

## SEAM — nambah deteksi TANPA buka frozen (plug-and-play)
`scanner/auditors.go` cuma DEKLARASI `var Auditors = map[string]AuditFunc{}`. Auditor baru =
**file sibling BARU** `scanner/auditors_<x>.go` yang isi `func init(){ Auditors["x"]=AuditX }`.
init() nambah ke map saat boot → runner otomatis dispatch. **Auditor yang SUDAH ADA
(arch/cwd/secrets/invariant/v2..v11) = FROZEN** (owner 2026-06-26: "scanner freeze bukan lock") →
deteksi lama gak bisa dirusak. Nambah deteksi = FILE BARU (map di-append via init, walau file lama
frozen). Pipa + semua auditor lama beku → evolusi nambah doang, GAK BISA robohin yang jalan.

## SWITCH
- `FLOWORK_SCANNER_AUTOSCAN` (bool, default ON, kategori "Security / Scanner") — auto-scan saat file
  berubah. OFF = file-watcher tetap idup tapi gak auto-scan; **scan manual + tool TETAP jalan**
  (security gak mati). Dibaca runtime di `codescan/engine.go` (frozen, baca `os.Getenv` → GUI-tunable).
- Allowlist exec/target (nmap/nuclei + domain owner) = DATA di `scan_allowlist` (GUI/endpoint), bukan kode.

## NON-FROZEN (seam, sengaja)
`web/tabs/scanner.js` (GUI), `feature_ops.go`/`feature_agents.go` (route registration), semua
`scanner/auditors_*.go` (zona deteksi), `internal/fwswitch/registry.go` (switch).

## VERIFIKASI 2026-06-26
QC live (login :1987): 116 auditor, arsenal 3-plane (auditor/tool/nuclei), allowlist owner-gated,
manual scan (run valid), auto-scan-on-filechange (run #1818 nyertok file repo → 9 temuan). Strip
token-identik (re-strip HEAD == current). Build+vet PASS. TestKernelFreeze PASS. Append → "Operation
not permitted".
