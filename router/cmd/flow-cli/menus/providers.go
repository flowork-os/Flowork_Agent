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

func Providers(c *api.Client) error {
	return utils.RunMenu("Providers", []utils.MenuItem{
		{Label: "List providers", Action: func() error { return listProviders(c) }},
		{Label: "Add provider", Action: func() error { return addProvider(c) }},
		{Label: "Toggle active", Action: func() error { return toggleProvider(c) }},
		{Label: "Delete provider", Action: func() error { return deleteProvider(c) }},
	})
}

type provider struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	IsActive bool   `json:"isActive"`
	Priority int    `json:"priority"`
}

func fetchProviders(c *api.Client) ([]provider, error) {
	var arr []provider
	if err := c.Get("/api/providers", &arr); err == nil {
		return arr, nil
	}
	var wrap struct {
		Data []provider `json:"data"`
	}
	if err := c.Get("/api/providers", &wrap); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

func listProviders(c *api.Client) error {
	arr, err := fetchProviders(c)
	if err != nil {
		return err
	}
	rows := make([][]string, len(arr))
	for i, p := range arr {
		active := "no"
		if p.IsActive {
			active = "yes"
		}
		rows[i] = []string{p.ID, p.Name, p.Provider, active, fmt.Sprintf("%d", p.Priority)}
	}
	utils.Table([]string{"id", "name", "provider", "active", "priority"}, rows)
	return nil
}

func addProvider(c *api.Client) error {
	name := utils.Prompt("Display name", "")
	if name == "" {
		return fmt.Errorf("name required")
	}
	provider := utils.Prompt("Provider (openai/anthropic/gemini/codex/cursor/kiro/azure/...)", "openai")
	authType := utils.Prompt("Auth type (apiKey/oauth/aws/none)", "apiKey")
	apiKey := utils.PromptSecret("API key")
	body := map[string]any{
		"name":     name,
		"provider": provider,
		"authType": authType,
		"isActive": true,
		"priority": 1,
		"data": map[string]any{
			"apiKey": apiKey,
		},
	}
	var resp map[string]any
	if err := c.Post("/api/providers", body, &resp); err != nil {
		return err
	}
	utils.Success("provider added")
	return nil
}

func toggleProvider(c *api.Client) error {
	arr, err := fetchProviders(c)
	if err != nil {
		return err
	}
	if len(arr) == 0 {
		utils.Warn("no providers")
		return nil
	}
	labels := make([]string, len(arr))
	for i, p := range arr {
		state := "OFF"
		if p.IsActive {
			state = "ON"
		}
		labels[i] = fmt.Sprintf("[%s] %s (%s)", state, p.Name, p.Provider)
	}
	idx := utils.Select("Pick provider", labels)
	if idx < 0 {
		return nil
	}
	pid := arr[idx].ID
	body := map[string]any{"isActive": !arr[idx].IsActive}
	if err := c.Put("/api/providers/"+pid, body, nil); err != nil {
		return err
	}
	utils.Success("toggled")
	return nil
}

func deleteProvider(c *api.Client) error {
	arr, err := fetchProviders(c)
	if err != nil {
		return err
	}
	if len(arr) == 0 {
		utils.Warn("no providers")
		return nil
	}
	labels := make([]string, len(arr))
	for i, p := range arr {
		labels[i] = fmt.Sprintf("%s (%s)", p.Name, p.Provider)
	}
	idx := utils.Select("Pick provider to delete", labels)
	if idx < 0 {
		return nil
	}
	if !utils.Confirm(fmt.Sprintf("Really delete %q?", arr[idx].Name), false) {
		return nil
	}
	if err := c.Delete("/api/providers/" + arr[idx].ID); err != nil {
		return err
	}
	utils.Success("deleted")
	return nil
}
