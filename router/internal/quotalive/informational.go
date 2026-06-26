// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package quotalive

import (
	"context"
	"fmt"
	"time"
)

func init() {
	Register(&informationalFetcher{
		vendor:  "iflow",
		message: "iFlow connected. No public usage API — quota tracked per request.",
	})
	Register(&informationalFetcher{
		vendor:  "qwen",
		message: "Qwen connected. No public usage API — quota tracked per request.",
	})
	Register(&informationalFetcher{
		vendor:  "ollama",
		message: "Ollama Cloud connected. No public usage API — free tier limits reset every 5h & 7d.",
	})
}

type informationalFetcher struct {
	vendor  string
	message string
}

func (f *informationalFetcher) Name() string { return f.vendor }

func (f *informationalFetcher) Fetch(ctx context.Context, p Params) (Snapshot, error) {
	if p.Token == "" {
		return Snapshot{}, fmt.Errorf("%s: token required", f.vendor)
	}
	return Snapshot{
		Provider:  f.vendor,
		Plan:      f.message,
		FetchedAt: time.Now().UTC(),
	}, nil
}
