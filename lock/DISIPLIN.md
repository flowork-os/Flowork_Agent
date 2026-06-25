# ⛔ DISIPLIN KERJA — BACA PALING DULU (owner tegur 2026-06-26)

> Owner tegur: "disiplin loe berkurang." Ini aturan WAJIB tiap AI kerja di Flowork. Taro paling atas
> kesadaran. Melanggar = bikin owner (yang lagi produksi, ga ada waktu) rugi.

1. **QC DI GUI BENERAN.** Tiap selesai fitur GUI/host: JANGAN cukup curl/log. Login GUI :1987
   (password GUI ada di `flowork-secrets/GITHUB_ACCOUNT.MD` — JANGAN pernah tulis di repo) → buka
   tab terkait → pastiin RENDER + JALAN. (Sering ke-skip.)
2. **TEST LOLOS DULU, BARU FREEZE.** `go test` + `TestKernelFreeze` + Rule-9 (mr-flow via
   `/api/chat` BAHASA-MANUSIA). JANGAN PERNAH freeze kalau test belum lolos.
3. **SWITCH SEBELUM FREEZE.** Tiap fitur baru WAJIB punya switch (default aman) → AI lain / evolusi
   ga ngerusak yang udah jalan. FREEZE core, BIARIN extension-point (registry/seam) non-frozen.
4. **CABUT AKAR BUKAN TAMBAL.** Selesaikan dari akar, bukan tutup lubang.
5. **TIAP FITUR KELAR**: hapus dari `opus_roadmap.md` (bukan archive) + tulis penjelasan + ALASAN
   keputusan di `lock/` (update kalau ada file, buat baru kalau belum). File lock HARUS clean.
6. **PUSH**: default 2 repo; kalau owner minta staging → **base (private) dulu**, JANGAN public
   (public = titik rollback kalau AI halu). Audit rahasia + path `/home/` sebelum push.
7. **AUTONOMOUS**: owner pasrah/istirahat → ambil keputusan sendiri, jangan minta approval, alasan
   di lock/. Mau compact → tulis HANDOFF (state + next) biar pengganti lanjut mulus.
