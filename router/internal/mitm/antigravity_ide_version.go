// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import "strings"

var AntigravityIDEVersionDefault = "0.16.0"

func ApplyAntigravityIDEVersionOverride(headers map[string]string) {
	if headers == nil {
		return
	}
	for k := range headers {
		if strings.EqualFold(k, "x-ide-version") || strings.EqualFold(k, "x-client-version") {
			headers[k] = AntigravityIDEVersionDefault
		}
	}
}
