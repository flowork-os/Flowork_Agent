// instinctenrich_ext.go — GROWTH-POINT (NON-frozen, JANGAN di-freeze).
//
// Pasangan extension buat instinctenrich.go (FROZEN). Flowork didesain BEREVOLUSI
// tanpa ngerusak diri: logika inti injeksi insting di-freeze (stabil, deterministik),
// TAPI cara MILIH insting bisa di-ganti/di-extend DI SINI tanpa buka freeze.
//
// Default: KOSONG → maybeInjectInstinct pakai rankInstincts bawaan (token-overlap,
// no-vindex, fails-open). Ini AMAN & udah kebukti live.
//
// Cara extend (TANPA unfreeze instinctenrich.go):
//   func init() {
//       RegisterInstinctSelector(func(all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer {
//           // contoh-contoh evolusi:
//           //  (a) RI-1 vindex idup → rank SEMANTIC (cosine) ganti token-overlap.
//           //  (b) #6 brain-as-service → scoping: agent LUAR (non-flowork) skip
//           //      room=instinct_tool (mereka punya tool sendiri; cuma dapet
//           //      insting UNIVERSAL/reasoning biar ga halu tool-Flowork).
//           //  (c) boost domain tertentu sesuai peran agent (koloni berlapis).
//           return rankInstincts(all, query, max) // fallback ke default
//       })
//   }
//
// Switch lain (ga perlu file ini): ENV FLOWORK_INSTINCT_INJECT=0 (matiin),
// FLOWORK_INSTINCT_INJECT_MAX=N (cap). Tumbuhin awareness = tambah drawer
// room=instinct_* di brain (ga sentuh kode sama sekali).

package router
