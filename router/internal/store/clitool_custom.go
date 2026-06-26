// clitool_custom.go — simpan definisi CLI tool CUSTOM (user-added dari GUI) di tabel kv,
// prefix "clitool_custom:". Value = JSON clitools.Tool (store ga import clitools → simpan raw JSON).
// NON-frozen (extension data layer).
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package store

import "database/sql"

const customCLIPrefix = "clitool_custom:"

// ListCustomCLITools — balikin semua JSON definisi CLI tool custom.
func ListCustomCLITools(d *sql.DB) ([]string, error) {
	rows, err := d.Query(`SELECT v FROM kv WHERE k LIKE ? ORDER BY k`, customCLIPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if rows.Scan(&v) == nil && v != "" {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// UpsertCustomCLITool — simpan/replace definisi (id = Tool.ID, jsonVal = JSON Tool).
func UpsertCustomCLITool(d *sql.DB, id, jsonVal string) error {
	_, err := d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, datetime('now'))
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
		customCLIPrefix+id, jsonVal)
	return err
}

// DeleteCustomCLITool — hapus definisi by id.
func DeleteCustomCLITool(d *sql.DB, id string) error {
	_, err := d.Exec(`DELETE FROM kv WHERE k = ?`, customCLIPrefix+id)
	return err
}
