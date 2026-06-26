// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/api"
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/menus"
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/utils"
)

func cmdUI(_ []string) error {
	c := api.New(rootURL, apiKey)
	utils.Header("flow_router — interactive control")
	utils.Info("URL: " + rootURL)
	return utils.RunMenu("Main menu", []utils.MenuItem{
		{Label: "Providers", Action: func() error { return menus.Providers(c) }},
		{Label: "API Keys", Action: func() error { return menus.APIKeys(c) }},
		{Label: "Combos", Action: func() error { return menus.Combos(c) }},
		{Label: "Settings", Action: func() error { return menus.Settings(c) }},
		{Label: "CLI Tools", Action: func() error { return menus.CLITools(c) }},
	})
}
