// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	ComboStrategyPriority    = "priority"
	ComboStrategyRoundRobin  = "round_robin"
	ComboStrategyRandom      = "random"
	ComboStrategyCostOptimal = "cost_optimal"
)

type Combo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Models    []string  `json:"models"`
	Strategy  string    `json:"strategy"`
	CreatedAt time.Time `json:"createdAt"`
}

func ListCombos(d *sql.DB) ([]Combo, error) {
	rows, err := d.Query(`SELECT id, name, models, strategy, createdAt FROM combos ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	var out []Combo
	for rows.Next() {
		var c Combo
		var modelsJSON, createdStr string
		if err := rows.Scan(&c.ID, &c.Name, &modelsJSON, &c.Strategy, &createdStr); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(modelsJSON), &c.Models)
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		out = append(out, c)
	}
	return out, nil
}

func GetComboByName(d *sql.DB, name string) (*Combo, error) {
	row := d.QueryRow(`SELECT id, name, models, strategy, createdAt FROM combos WHERE name = ?`, name)
	var c Combo
	var modelsJSON, createdStr string
	if err := row.Scan(&c.ID, &c.Name, &modelsJSON, &c.Strategy, &createdStr); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(modelsJSON), &c.Models)
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &c, nil
}

func UpsertCombo(d *sql.DB, c *Combo) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
		c.CreatedAt = time.Now().UTC()
	}
	if c.Strategy == "" {
		c.Strategy = ComboStrategyPriority
	}
	modelsJSON, _ := json.Marshal(c.Models)
	_, err := d.Exec(`INSERT INTO combos (id, name, models, strategy, createdAt) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, models=excluded.models, strategy=excluded.strategy`,
		c.ID, c.Name, string(modelsJSON), c.Strategy, c.CreatedAt.Format(time.RFC3339))
	return err
}

func DeleteCombo(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM combos WHERE id = ?`, id)
	return err
}
