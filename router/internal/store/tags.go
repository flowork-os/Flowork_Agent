// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
}

func ListTags(d *sql.DB) ([]Tag, error) {
	rows, err := d.Query(`SELECT id, name, color, kind, createdAt FROM tags ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		var ts string
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Kind, &ts); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, t)
	}
	return out, nil
}

func UpsertTag(d *sql.DB, t *Tag) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
		t.CreatedAt = time.Now().UTC()
	}
	if t.Color == "" {
		t.Color = "#8b5cf6"
	}
	if t.Kind == "" {
		t.Kind = "generic"
	}
	_, err := d.Exec(`INSERT INTO tags (id, name, color, kind, createdAt)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, color=excluded.color, kind=excluded.kind`,
		t.ID, t.Name, t.Color, t.Kind, t.CreatedAt.Format(time.RFC3339))
	return err
}

func DeleteTag(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM tags WHERE id = ?`, id)
	return err
}
