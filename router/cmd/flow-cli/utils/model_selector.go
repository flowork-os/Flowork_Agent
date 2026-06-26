// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package utils

import (
	"sort"

	"github.com/flowork-os/flowork_Router/cmd/flow-cli/api"
)

func PickModel(c *api.Client) string {
	var wrap struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := c.Get("/v1/models", &wrap); err != nil {
		Error("/v1/models: " + err.Error())
		return ""
	}
	if len(wrap.Data) == 0 {
		Warn("no models available")
		return ""
	}
	sort.Slice(wrap.Data, func(i, j int) bool { return wrap.Data[i].ID < wrap.Data[j].ID })
	labels := make([]string, len(wrap.Data))
	for i, m := range wrap.Data {
		labels[i] = m.ID + "   (" + m.OwnedBy + ")"
	}
	idx := Select("Pick model", labels)
	if idx < 0 {
		return ""
	}
	return wrap.Data[idx].ID
}
