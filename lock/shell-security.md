# SHELL SECURITY — classifier bahaya (AKAR: substring → terstruktur)

> Gap F-B (ROADMAP): deny-list shell lama pakai `strings.Contains` substring naif →
> gampang di-bypass (`rm  -rf   /` spasi-dobel, `rm --recursive --force /` beda ejaan,
> kapital). Fix: classifier ternormalisasi + struktural, ZERO false-positive.

## FILE
| File | Peran | Status |
|---|---|---|
| `agent/internal/shellsec/shellsec.go` | `Dangerous(cmd) (bool,reason)` — normalisasi whitespace + deteksi `rm` recursive+force ke PATH SISTEM (flag-order/ejaan independent) + fork-bomb regex + danger-list ternormalisasi. Security primitive. | **FROZEN** (2026-07-02) |
| `agent/internal/tools/builtins/shell.go` | `bashTool.Run` panggil `shellsec.Dangerous` setelah deny-list lama (additive). | **FROZEN** (re-freeze) |
| `agent/internal/kernel/runtime/host.go` | raw host-exec panggil `shellsec.Dangerous` setelah `hostExecDeny` (defence-in-depth). | **FROZEN** (re-freeze) |
| `agent/internal/shellsec/shellsec_test.go` | Bukti nangkep bypass (spasi/kapital/ejaan/subcommand) + TIDAK over-block relatif (`rm -rf ./build`, `rm -rf *`). | test |

## DESAIN (kenapa ZERO false-positive)
Cuma blokir yang JELAS katastrofik: `rm` recursive+force ke path ABSOLUT-sistem (`/`, `~`,
`/etc`, `/usr`, …) atau `$HOME`. Path RELATIF (`.`, `*`, `./build`) TIDAK diblok → mr-flow
tetep bisa kerja normal (hapus build-dir dll). Plus mkfs/dd-ke-disk/fork-bomb/shutdown-family/
baca-shadow. Additive di atas deny-list lama (ga ngurangin proteksi, cuma nambah).

## CATATAN gap F-B lain
- **Approval-gate interaktif** (mode default/plan/accept/bypass): SENGAJA ga dibikin — bentrok
  sama desain otonom Flowork ("putusin sendiri, jangan banyak nanya"). Proteksi yang pas =
  hard-block katastrofik (classifier ini), sisanya jalan otonom.
- **Fail-closed unknown-tool**: udah ada via broker-cap + owner-allowlist (tool tak dikenal ga
  nyampe exec). Lihat komentar `host.go`.
