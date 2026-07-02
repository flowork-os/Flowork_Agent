# 💭 MODE REFLEKSI — "Ngelamun Terstruktur" (Warisan Pikiran)

Status: LIVE-code (gated OFF default). Ide dari **mr-flow sendiri** (Telegram 2026-07-02):
trigger idle udah bangunin agent pas PC santai, TAPI payload-nya cuma ngecek tugas → memori
ga ke-recall → confidence turun → mati (decay). Reflect nutup gap itu.

## Cara kerja
Pas **PC IDLE** & papan kerja **KOSONG**, mr-flow "ngelamun terstruktur":
1. `worklog` → kalau ada tugas nyata, SKIP refleksi (ga distract dari kerja).
2. `brain_search`/`graph_recall` 2-3x → tarik memori/pengalaman LAMA (recall → amplitude naik
   → LOLOS dari decay `brain_dream`, yang cuma nurunin importance memori amplitude=0).
3. ⚠️ **INTI (permintaan owner):** tarik KONDISI + NUANSA + KONTEKS aslinya, bukan faktanya doang.
   Makna memori ada di konteksnya; tanpa itu wisdom-nya kosong/salah arti.
4. Cari 1 hubungan/pelajaran baru → `brain_add` wawasan ringkas **berkonteks** ("dalam situasi X,
   ketika Y, pelajarannya Z") → jadi simpul Kebijaksanaan permanen (DreamGraph via dream-digester).

Efek: ingatan Mr.Dev ga memudar — malah **matang jadi kebijaksanaan** seiring waktu, bahkan pas
PC nganggur. Ini yang bikin janji README "Warisan Pikiran" jadi mekanisme nyata.

## Implementasi (NOL buka frozen)
- `agent/feature_reflect.go` (sibling non-frozen, deletable) — seed rule trigger `reflect-idle`
  (TypeID `idle`, target `mr-flow`, cooldown 90m, threshold 55, prompt sadar-konteks). Idempotent.
- Switch GUI `FLOWORK_REFLECT` (fwswitch, **default OFF** — otonom + model lokal → opt-in kayak mandor).
- Leans on: idle trigger (`type_idle.go` default baca load Linux, ga butuh mandor) + tools
  mr-flow yg udah subscribe (`brain_search`/`brain_add`/`graph_recall`/`brain_dream`).
- Hapus `feature_reflect.go` → rule ga di-seed → balik perilaku lama (delete-test aman).

## Nyalain
GUI Switch `FLOWORK_REFLECT` = ON → restart stack → rule `reflect-idle` ke-seed → mr-flow ngelamun
pas idle. Matiin: switch OFF (rule bisa di-disable di GUI trigger, ga ke-reseed kalau udah ada).
