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

type TranslatorDraft struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SourceFormat string    `json:"sourceFormat"`
	TargetFormat string    `json:"targetFormat"`
	Input        string    `json:"input"`
	Output       string    `json:"output"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func ListTranslatorDrafts(d *sql.DB) ([]TranslatorDraft, error) {
	rows, err := d.Query(`SELECT id, COALESCE(name, ''), sourceFormat, targetFormat, COALESCE(input, ''), COALESCE(output, ''), createdAt, updatedAt FROM translatorDrafts ORDER BY updatedAt DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TranslatorDraft
	for rows.Next() {
		var t TranslatorDraft
		var c, u string
		if err := rows.Scan(&t.ID, &t.Name, &t.SourceFormat, &t.TargetFormat, &t.Input, &t.Output, &c, &u); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, c)
		t.UpdatedAt, _ = time.Parse(time.RFC3339, u)
		out = append(out, t)
	}
	return out, nil
}

func GetTranslatorDraft(d *sql.DB, id string) (*TranslatorDraft, error) {
	row := d.QueryRow(`SELECT id, COALESCE(name, ''), sourceFormat, targetFormat, COALESCE(input, ''), COALESCE(output, ''), createdAt, updatedAt FROM translatorDrafts WHERE id = ?`, id)
	var t TranslatorDraft
	var c, u string
	err := row.Scan(&t.ID, &t.Name, &t.SourceFormat, &t.TargetFormat, &t.Input, &t.Output, &c, &u)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, c)
	t.UpdatedAt, _ = time.Parse(time.RFC3339, u)
	return &t, nil
}

func UpsertTranslatorDraft(d *sql.DB, t *TranslatorDraft) error {
	now := time.Now().UTC()
	if t.ID == "" {
		t.ID = uuid.NewString()
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	_, err := d.Exec(`INSERT INTO translatorDrafts (id, name, sourceFormat, targetFormat, input, output, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			sourceFormat=excluded.sourceFormat,
			targetFormat=excluded.targetFormat,
			input=excluded.input,
			output=excluded.output,
			updatedAt=excluded.updatedAt`,
		t.ID, t.Name, t.SourceFormat, t.TargetFormat, t.Input, t.Output, t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339))
	return err
}

func DeleteTranslatorDraft(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM translatorDrafts WHERE id = ?`, id)
	return err
}
