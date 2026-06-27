// cognitive_ext.go — TITIK EXTENSION (NON-FROZEN, BISA DIHAPUS) buat Cognitive Graph (CGM).
//
// ⚖️ ATURAN ABADI (owner Mr.Dev): file CGM frozen (cognitive_handlers.go, cognitive_tensions.go)
// TIDAK BOLEH dibuka buat nambah filtur. Switch-reader (node/edge limit) udah pindah ke
// cognitive_seam.go (BEKU, default aman) biar inti self-sufficient. Tuning lewat ENV
// (FLOWORK_CGM_*) — ga perlu edit kode. Hook/filtur CGM BARU: tambah di sini (non-frozen).
//
// 📖 WAJIB BACA: FLowork_os/lock/CognitiveGraph.md sebelum ngutak-atik CGM.
package agentmgr
