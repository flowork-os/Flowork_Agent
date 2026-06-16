// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-16 (LOCKED ≠ FREEZE).
// AI lain: JANGAN otak-atik tanpa izin owner. Teruji (TestBuilderMatchesBuild).
package vecindex

import "errors"

// builder.go — STREAMING builder buat korpus GEDE (5jt drawer) yang gak muat di RAM sebagai fp32
// (5jt × 1024 × 4B = 20GB). Build() biasa nelen semua [][]float32 sekaligus → OOM. Builder ini
// nguantisasi tiap vektor pas di-Add (cuma nyimpen kode int8 yg numbuh = ~1B/dim) → memori konstan
// (~ukuran index akhir doang, ~1B/dim). File terpisah dari vecindex.go (locked) — owner-approved
// RAG feature 2026-06-16. Akses quantizeInto + field Index (sepaket).
//
// Pakai (2-pass, scale optimal): pass-1 scan korpus → max|komponen| → NewBuilder(dim, scale) →
// pass-2 Add tiap vektor (stream dari disk) → Finish() → Index. Save ke disk.

// Builder — akumulator index streaming. scale = max|komponen| (skala kuantisasi global, ditentuin
// caller; idealnya hasil scan pass-1 biar full-range = recall optimal).
type Builder struct {
	dim   int
	scale float32
	codes []int8
	ids   []string
}

// NewBuilder — builder kosong. dim = dimensi vektor; scale = max|komponen| (>0; ≤0 → 1).
func NewBuilder(dim int, scale float32) *Builder {
	if scale <= 0 {
		scale = 1
	}
	return &Builder{dim: dim, scale: scale}
}

// Add — kuantisasi v (len==dim, idealnya unit-normalized) ke kode 8-bit + append. id = label.
func (b *Builder) Add(id string, v []float32) error {
	if len(v) != b.dim {
		return errors.New("vecindex: Builder.Add dim mismatch")
	}
	start := len(b.codes)
	b.codes = append(b.codes, make([]int8, b.dim)...)
	quantizeInto(v, b.scale, b.codes[start:])
	b.ids = append(b.ids, id)
	return nil
}

// Len — jumlah vektor yang udah di-Add.
func (b *Builder) Len() int { return len(b.ids) }

// Finish — kunci jadi Index immutable (siap Search / Save).
func (b *Builder) Finish() *Index {
	return &Index{dim: b.dim, scale: b.scale, codes: b.codes, ids: b.ids}
}
