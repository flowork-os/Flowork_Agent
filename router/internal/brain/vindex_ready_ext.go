// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN 2026-07-02 (owner) — jangan edit. Helper 1-fungsi, stabil. 📄 Dok: lock/prompt-diet.md
// vindex_ready_ext.go — expose kesiapan index vektor semantic ke luar package,
// buat seam enrichment-selektif di router. Ga nyentuh frozen semantic.go /
// semantic_threshold_ext.go.

package brain

// VectorIndexReady — true kalau index vektor semantic udah kebangun & siap dipakai.
// Dipakai seam enrichment selektif: index belum siap → caller fallback ke retrieve
// lama (SemanticRetrieve) biar perilaku ga berubah di mesin tanpa index.
func VectorIndexReady() bool { return loadVIndex() != nil }
