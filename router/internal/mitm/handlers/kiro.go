// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package handlers

import "net/http"

func init() { Register(&kiroHandler{}) }

type kiroHandler struct{}

func (k *kiroHandler) Name() string { return "kiro" }

func (k *kiroHandler) Handle(w http.ResponseWriter, r *http.Request) {
	r.Header.Del("x-amzn-codewhisperer-profile-arn")
	r.Header.Del("amz-sdk-invocation-id")
	rerouteToRouter(w, r, "/v1/chat/completions")
}
