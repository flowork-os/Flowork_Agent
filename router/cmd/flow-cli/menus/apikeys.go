// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package menus

import (
	"fmt"

	"github.com/flowork-os/flowork_Router/cmd/flow-cli/api"
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/utils"
)

func APIKeys(c *api.Client) error {
	return utils.RunMenu("API keys", []utils.MenuItem{
		{Label: "List keys", Action: func() error { return listKeys(c) }},
		{Label: "Create key", Action: func() error { return createKey(c) }},
		{Label: "Revoke key", Action: func() error { return revokeKey(c) }},
	})
}

type apiKey struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	KeyPrefix     string  `json:"keyPrefix"`
	IsActive      bool    `json:"isActive"`
	DailyCapUsd   float64 `json:"dailyCapUsd"`
	MonthlyCapUsd float64 `json:"monthlyCapUsd"`
	Plaintext     string  `json:"plaintext,omitempty"`
}

func fetchKeys(c *api.Client) ([]apiKey, error) {
	var wrap struct {
		Data []apiKey `json:"data"`
	}
	if err := c.Get("/api/keys", &wrap); err == nil && wrap.Data != nil {
		return wrap.Data, nil
	}
	var arr []apiKey
	if err := c.Get("/api/keys", &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func listKeys(c *api.Client) error {
	arr, err := fetchKeys(c)
	if err != nil {
		return err
	}
	rows := make([][]string, len(arr))
	for i, k := range arr {
		active := "no"
		if k.IsActive {
			active = "yes"
		}
		rows[i] = []string{k.ID, k.Name, k.KeyPrefix, active, utils.USD(k.DailyCapUsd), utils.USD(k.MonthlyCapUsd)}
	}
	utils.Table([]string{"id", "name", "prefix", "active", "dailyCap", "monthlyCap"}, rows)
	return nil
}

func createKey(c *api.Client) error {
	name := utils.Prompt("Key name", "")
	if name == "" {
		return fmt.Errorf("name required")
	}
	daily := utils.PromptInt("Daily cap USD (0 = unlimited)", 0)
	monthly := utils.PromptInt("Monthly cap USD (0 = unlimited)", 0)
	body := map[string]any{
		"name":          name,
		"isActive":      true,
		"dailyCapUsd":   daily,
		"monthlyCapUsd": monthly,
	}
	var out apiKey
	if err := c.Post("/api/keys", body, &out); err != nil {
		return err
	}
	utils.Success("Created. Plaintext (copy now — server will not show it again):")
	fmt.Println("  " + out.Plaintext)
	if utils.Confirm("Copy to clipboard?", true) {
		if err := utils.Copy(out.Plaintext); err == nil {
			utils.Success("copied")
		}
	}
	return nil
}

func revokeKey(c *api.Client) error {
	arr, err := fetchKeys(c)
	if err != nil {
		return err
	}
	if len(arr) == 0 {
		utils.Warn("no keys")
		return nil
	}
	labels := make([]string, len(arr))
	for i, k := range arr {
		labels[i] = fmt.Sprintf("%s (%s)", k.Name, k.KeyPrefix)
	}
	idx := utils.Select("Pick key to revoke", labels)
	if idx < 0 {
		return nil
	}
	if !utils.Confirm(fmt.Sprintf("Revoke %q?", arr[idx].Name), false) {
		return nil
	}
	return c.Delete("/api/keys/" + arr[idx].ID)
}
