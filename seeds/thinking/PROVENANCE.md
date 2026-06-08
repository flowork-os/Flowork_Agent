# Provenance — corpus bahan ingest group `thinking` (CATATAN INTERNAL, JANGAN di-ingest)

> File ini **bukan** data ingest. Ini catatan sumber buat kejujuran legal/audit kita.
> Data yang masuk brain (`strategy.jsonl`, `improvement.jsonl`) sudah **brand-neutral**:
> nol nama merek/penulis/sitiran — cuma polanya. Diverifikasi via leak-check (0 hit).

## strategy.jsonl — 379 pola
- **Sumber**: teks klasik strategi, terjemahan Lionel Giles (1910).
- **Status hukum**: **public domain** (Project Gutenberg eBook #132, rilis 1994; "no cost, almost no restrictions"). PD ga butuh atribusi — aman di-brand-neutral.
- **Proses**: ambil teks asli → buang komentar penerjemah (blok `[...]`) → pecah per-ayat → buang sapaan/nama penulis ("… said:", nama strategist) → simpan teks pola murni.
- **Anti-halu**: isi = teks asli verbatim (bukan ringkasan model), cuma labelnya yang dicabut.

## improvement.jsonl — 30 pola
- **Sumber dasar (buat grounding distilasi)**: artikel pengetahuan publik proses-perbaikan — Wikipedia *Kaizen*, *PDCA*, *5S*, *Lean manufacturing*, *Five whys* (CC BY-SA).
- **Status hukum**: prinsip/fakta **tidak bisa di-hak-cipta**; pernyataan ditulis ulang **dengan kata-kata kita sendiri** (bukan salinan teks), jadi ga butuh atribusi & aman brand-neutral. Buku kanon Kaizen (Masaaki Imai, 1986) **tidak** dipakai/disalin (hak cipta).
- **Proses**: baca sumber otoritatif → distill jadi pernyataan-prinsip netral → buang semua jargon/merek ("kaizen/toyota/jepang/lean/pdca/5s/gemba/muda/mura/muri/deming") → verifikasi 0 bocoran.

## Skema record (dua file sama)
```json
{"id":"strategy-0001","lens":"strategy","pattern":"<teks pola, brand-neutral>"}
{"id":"improvement-0001","lens":"improvement","pattern":"<teks pola, brand-neutral>"}
```
`lens` = penanda fungsi netral (bukan merek). Tiap agent punya brain terpisah, jadi
pola strategi masuk ke brain agent-strategi, pola perbaikan ke brain agent-perbaikan.

## Cara ingest nanti (saat agent udah ada)
Per agent: baca file → tiap baris → `store.brain.add {content: pattern, wing: "doctrine"}`.
Kontrak anti-halu di prompt agent: **jawab hanya dari hasil `store.brain.search`; kalau ga ketemu, bilang ga ada dasarnya — dilarang ngarang.**
