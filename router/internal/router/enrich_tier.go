// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"os"
	"strings"
)

func isCrewLightModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	if v := strings.TrimSpace(os.Getenv("FLOW_ROUTER_LIGHT_MODELS")); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(strings.ToLower(s)); s != "" && strings.Contains(model, s) {
				return true
			}
		}
		return false
	}
	return strings.Contains(model, "haiku")
}
