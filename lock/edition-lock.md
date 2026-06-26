# EDITION LOCK (#2) — FREE (anti-rebrand) vs CORPORATE (white-label)

> Owner: Aola Sahidin (Mr.Dev) · 2026-06-26. Build FREE = Mr.Flow AS-IS, identitas (persona Aola +
> konstitusi AOLA) DIKUNCI biar ga ada yg rebrand+jual-ulang. CORPORATE (berbayar) = unlock white-label.

## MEKANISME (#2 bagian 1 — SELESAI)
`edition_gate.go` (NON-frozen) `editionGate(handler)`: di edisi FREE, WRITE (POST/PUT/DELETE/PATCH)
ke endpoint IDENTITAS ditolak **403** (anti-rebrand); READ (GET) tetep jalan (user boleh LIHAT, ga
boleh UBAH). CORPORATE = full akses.

**Endpoint yg dikunci** (di-wrap di `routes.go`): `/api/brain/constitution`, `/api/brain/personas`,
`/api/brain/constitution/{propose,vote,amend,amend/vote}`. (Konstitusi udah memuat AOLA-001_IDENTITAS
= persona Aola → kunci konstitusi = kunci identitas.)

**Switch `FLOWORK_EDITION`** (GUI Switch Fitur kategori "Bisnis / Edition", prefix FLOWORK_ →
fwswitch lintas-proses): default **`free`** (terkunci) · `corporate` (unlock). Default-deny = aman
(build tanpa set apa-apa = FREE-locked).

## VERIFIKASI (live)
- FREE: POST constitution → **403 locked**; GET constitution → **200** (read jalan); POST personas → **403**.
- CORPORATE (set via GUI file): POST personas → handler jalan (500 "name required", BUKAN 403) = unlock ✓.
- Revert FREE → 403 lagi ✓. Switch GUI cross-process kebukti.

## SISA (#2 bagian 2 — FASE OWNER, bukan engineering murni)
- **Licensing**: sekarang `FLOWORK_EDITION=corporate` cuma env/switch → user teknis bisa flip sendiri.
  Buat lock berbayar BENERAN: corporate-unlock harus butuh **LICENSE KEY ber-tanda-tangan** (verifikasi
  signature), bukan cuma env. Roadmap bilang "fitur produk + lisensi, fase tersendiri" → keputusan +
  infra lisensi = owner. Mekanisme gate udah SIAP nyambung ke license-check (ganti `freeEdition()`
  jadi cek lisensi).
- **Build CORPORATE**: tinggal ship dgn `FLOWORK_EDITION=corporate` (+ kelak license-gated).

## CATATAN
- `edition_gate.go` + `routes.go` = NON-frozen (routes butuh nambah handler; gate kecil). Enforcement =
  switch default-FREE. Kalau mau anti-tamper lebih kuat → freeze + license-key (fase lisensi).
- Identitas inti (konstitusi AOLA-001..013) udah ke-lock dari WRITE; persona kv mr-flow (host :1987)
  = identitas runtime, bisa di-gate serupa di host kalau perlu (fase lanjut).
