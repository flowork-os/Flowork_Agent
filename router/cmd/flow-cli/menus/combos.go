// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package menus

import (
	"fmt"
	"strings"

	"github.com/flowork-os/flowork_Router/cmd/flow-cli/api"
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/utils"
)

func Combos(c *api.Client) error {
	return utils.RunMenu("Combos", []utils.MenuItem{
		{Label: "List combos", Action: func() error { return listCombos(c) }},
		{Label: "Create combo", Action: func() error { return createCombo(c) }},
		{Label: "Delete combo", Action: func() error { return deleteCombo(c) }},
	})
}

type combo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Strategy string   `json:"strategy"`
	Models   []string `json:"models"`
}

func fetchCombos(c *api.Client) ([]combo, error) {
	var wrap struct {
		Data []combo `json:"data"`
	}
	if err := c.Get("/api/combos", &wrap); err == nil && wrap.Data != nil {
		return wrap.Data, nil
	}
	var arr []combo
	if err := c.Get("/api/combos", &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func listCombos(c *api.Client) error {
	arr, err := fetchCombos(c)
	if err != nil {
		return err
	}
	rows := make([][]string, len(arr))
	for i, cb := range arr {
		rows[i] = []string{cb.ID, cb.Name, cb.Strategy, strings.Join(cb.Models, ", ")}
	}
	utils.Table([]string{"id", "name", "strategy", "models"}, rows)
	return nil
}

func createCombo(c *api.Client) error {
	name := utils.Prompt("Combo name", "")
	if name == "" {
		return fmt.Errorf("name required")
	}
	stratIdx := utils.Select("Strategy", []string{"priority", "round_robin", "random", "cost_optimal"})
	strats := []string{"priority", "round_robin", "random", "cost_optimal"}
	if stratIdx < 0 {
		stratIdx = 0
	}
	models := []string{}
	for i := 0; i < 10; i++ {
		m := utils.PickModel(c)
		if m == "" {
			break
		}
		models = append(models, m)
		if !utils.Confirm("Add another?", true) {
			break
		}
	}
	if len(models) == 0 {
		return fmt.Errorf("at least 1 model required")
	}
	body := map[string]any{"name": name, "strategy": strats[stratIdx], "models": models}
	if err := c.Post("/api/combos", body, nil); err != nil {
		return err
	}
	utils.Success("combo created")
	return nil
}

func deleteCombo(c *api.Client) error {
	arr, err := fetchCombos(c)
	if err != nil {
		return err
	}
	if len(arr) == 0 {
		utils.Warn("no combos")
		return nil
	}
	labels := make([]string, len(arr))
	for i, cb := range arr {
		labels[i] = cb.Name + " (" + cb.Strategy + ")"
	}
	idx := utils.Select("Pick combo to delete", labels)
	if idx < 0 {
		return nil
	}
	if !utils.Confirm("Delete?", false) {
		return nil
	}
	return c.Delete("/api/combos/" + arr[idx].ID)
}
