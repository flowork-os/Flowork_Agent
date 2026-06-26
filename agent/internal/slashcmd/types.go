// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package slashcmd

import "context"

const AlgoVersion = "v1"

type Result struct {
	Text   string `json:"text"`
	Format string `json:"format,omitempty"`
}

type SlashCommand interface {
	Name() string

	Aliases() []string

	Description() string

	Run(ctx context.Context, argsRaw string) (Result, error)
}
