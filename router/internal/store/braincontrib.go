// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"time"
)

type BrainContribution struct {
	ID       int64  `json:"id"`
	TS       string `json:"ts"`
	Agent    string `json:"agent"`
	Model    string `json:"model"`
	Mode     string `json:"mode"`
	Query    string `json:"query"`
	Sources  string `json:"sources"`
	Answer   string `json:"answer"`
	Ingested bool   `json:"ingested"`
}

func AddBrainContribution(d *sql.DB, c BrainContribution) error {
	_, err := d.Exec(`INSERT INTO brainContributions (ts, agent, model, mode, query, sources, answer, ingested)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		time.Now().UTC().Format(time.RFC3339), c.Agent, c.Model, c.Mode, c.Query, c.Sources, c.Answer)
	return err
}

func ListBrainContributions(d *sql.DB, pendingOnly bool, limit int) ([]BrainContribution, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT id, ts, agent, model, mode, query, sources, answer, ingested FROM brainContributions`
	if pendingOnly {
		q += ` WHERE ingested = 0`
	}
	q += ` ORDER BY id DESC LIMIT ?`
	rows, err := d.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BrainContribution
	for rows.Next() {
		var c BrainContribution
		var ing int
		if err := rows.Scan(&c.ID, &c.TS, &c.Agent, &c.Model, &c.Mode, &c.Query, &c.Sources, &c.Answer, &ing); err != nil {
			continue
		}
		c.Ingested = ing == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func CountBrainContributions(d *sql.DB) (total, pending int) {
	_ = d.QueryRow(`SELECT COUNT(*) FROM brainContributions`).Scan(&total)
	_ = d.QueryRow(`SELECT COUNT(*) FROM brainContributions WHERE ingested = 0`).Scan(&pending)
	return total, pending
}

func MarkContributionsIngested(d *sql.DB, maxID int64) (int64, error) {
	res, err := d.Exec(`UPDATE brainContributions SET ingested = 1 WHERE id <= ? AND ingested = 0`, maxID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
