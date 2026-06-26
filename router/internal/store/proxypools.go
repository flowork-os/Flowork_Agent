// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	ProxyRotationRoundRobin = "round-robin"
	ProxyRotationRandom     = "random"
	ProxyRotationSticky     = "sticky"
)

type ProxyPool struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Proxies   []string  `json:"proxies"`
	Rotation  string    `json:"rotation"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

func ListProxyPools(d *sql.DB) ([]ProxyPool, error) {
	rows, err := d.Query(`SELECT id, name, proxies, rotation, isActive, createdAt FROM proxyPools ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProxyPool
	for rows.Next() {
		var p ProxyPool
		var proxiesJSON, createdStr string
		var active int
		if err := rows.Scan(&p.ID, &p.Name, &proxiesJSON, &p.Rotation, &active, &createdStr); err != nil {
			return nil, err
		}
		p.IsActive = active == 1
		_ = json.Unmarshal([]byte(proxiesJSON), &p.Proxies)
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		out = append(out, p)
	}
	return out, nil
}

func UpsertProxyPool(d *sql.DB, p *ProxyPool) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = time.Now().UTC()
	}
	if p.Rotation == "" {
		p.Rotation = ProxyRotationRoundRobin
	}
	proxiesJSON, _ := json.Marshal(p.Proxies)
	active := 0
	if p.IsActive {
		active = 1
	}
	_, err := d.Exec(`INSERT INTO proxyPools (id, name, proxies, rotation, isActive, createdAt) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, proxies=excluded.proxies, rotation=excluded.rotation, isActive=excluded.isActive`,
		p.ID, p.Name, string(proxiesJSON), p.Rotation, active, p.CreatedAt.Format(time.RFC3339))
	return err
}

func DeleteProxyPool(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM proxyPools WHERE id = ?`, id)
	return err
}
