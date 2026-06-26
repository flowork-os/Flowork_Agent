// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import "github.com/flowork-os/flowork_Router/internal/caveman"

func injectCavemanIntoRequest(req *OpenAIRequest, level string) {
	prompt := caveman.Prompt(caveman.Normalize(level))
	if prompt == "" {
		return
	}
	for i := range req.Messages {
		if req.Messages[i].Role == "system" || req.Messages[i].Role == "developer" {
			req.Messages[i].Content = caveman.InjectIntoSystem(req.Messages[i].Content, prompt)
			return
		}
	}

	req.Messages = append([]OpenAIMessage{{Role: "system", Content: prompt}}, req.Messages...)
}
