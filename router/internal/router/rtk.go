// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"github.com/flowork-os/flowork_Router/internal/rtk"
)

const rtkToolResultCap = 4000

func compressMessagesRTK(msgs []OpenAIMessage) ([]OpenAIMessage, int) {
	out := make([]OpenAIMessage, len(msgs))
	copy(out, msgs)
	saved := 0
	for i := range out {
		if out[i].Role != "tool" {
			continue
		}
		c, n := rtk.Compress(out[i].Content, rtkToolResultCap)
		if n > 0 {
			out[i].Content = c
			saved += n
		}
	}
	return out, saved
}
