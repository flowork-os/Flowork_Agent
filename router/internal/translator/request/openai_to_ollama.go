// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package request

import "github.com/flowork-os/flowork_Router/internal/translator"

func init() {
	translator.Register(translator.Pair{From: "openai", To: "ollama"}, translator.DirRequest, OpenAIToOllama)
}

func OpenAIToOllama(body map[string]any) map[string]any {
	out := map[string]any{
		"model":    body["model"],
		"messages": body["messages"],
	}
	if v, ok := body["stream"]; ok {
		out["stream"] = v
	}
	opts := map[string]any{}
	if v, ok := body["temperature"]; ok {
		opts["temperature"] = v
	}
	if v, ok := body["top_p"]; ok {
		opts["top_p"] = v
	}
	if v, ok := body["max_tokens"]; ok {
		opts["num_predict"] = v
	}
	if len(opts) > 0 {
		out["options"] = opts
	}
	return out
}
