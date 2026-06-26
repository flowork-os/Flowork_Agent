// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package menus

import (
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/api"
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/utils"
)

func Settings(c *api.Client) error {
	return utils.RunMenu("Settings", []utils.MenuItem{
		{Label: "Show settings JSON", Action: func() error { return showSettings(c) }},
		{Label: "Toggle requireApiKey", Action: func() error { return toggleSetting(c, "requireApiKey") }},
		{Label: "Toggle requireLogin", Action: func() error { return toggleSetting(c, "requireLogin") }},
		{Label: "Toggle RtkTokenSaver", Action: func() error { return toggleSetting(c, "RtkTokenSaver") }},
	})
}

func showSettings(c *api.Client) error {
	var s map[string]any
	if err := c.Get("/api/settings", &s); err != nil {
		return err
	}
	utils.Info(utils.PrettyJSON(s))
	return nil
}

func toggleSetting(c *api.Client, key string) error {
	var s map[string]any
	if err := c.Get("/api/settings", &s); err != nil {
		return err
	}
	cur, _ := s[key].(bool)
	patch := map[string]any{key: !cur}
	if err := c.Put("/api/settings", patch, nil); err != nil {
		return err
	}
	utils.Success(key + " is now " + boolStr(!cur))
	return nil
}

func boolStr(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}
