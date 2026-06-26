// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package vecindex

import "errors"

type Builder struct {
	dim   int
	scale float32
	codes []int8
	ids   []string
}

func NewBuilder(dim int, scale float32) *Builder {
	if scale <= 0 {
		scale = 1
	}
	return &Builder{dim: dim, scale: scale}
}

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

func (b *Builder) Len() int { return len(b.ids) }

func (b *Builder) Finish() *Index {
	return &Index{dim: b.dim, scale: b.scale, codes: b.codes, ids: b.ids}
}
