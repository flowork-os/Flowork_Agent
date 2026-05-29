package db

// workspace_meta.go — seed + helper untuk tabel workspace_meta.
//
// Per Ayah 2026-04-25: semua doktrin di DB (single source of truth).
// Konten README.md + experience-log.md per task workspace di-migrate
// ke tabel workspace_meta. File .md masih ada (transisional, akan
// dihapus setelah 1 bulan stabil), tapi tools warga harus baca dari DB.
//
// Beda dengan seedEducationalErrors yang hardcoded di kode:
// SeedWorkspaceMeta scan folder workspaces/ dan baca file existing —
// because content workspace metadata ditulis warga sendiri (README +
// experience log), bukan ditentukan developer.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SeedWorkspaceMeta scan folder `<workspaceRoot>/workspaces/<task>/` dan
// migrate konten README.md + experience-log.md ke tabel workspace_meta.
//
// Idempotent via INSERT OR IGNORE — entry yang udah ada (mungkin
// ke-edit Ayah via GUI nanti) TIDAK ke-overwrite. Cuma task workspace
// baru yang masuk DB. Buat re-sync isi DB dari file .md, panggil
// UpdateWorkspaceMetaFromFile (belum di-implement — follow-up).
func SeedWorkspaceMeta(db *sql.DB, workspaceRoot string) error {
	wsDir := filepath.Join(workspaceRoot, "workspaces")
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		// Workspace dir belum ada (e.g. fresh install) — bukan error fatal.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read workspaces dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		readmePath := filepath.Join(wsDir, name, "README.md")
		logPath := filepath.Join(wsDir, name, "experience-log.md")
		readme := readFileOrEmpty(readmePath)
		expLog := readFileOrEmpty(logPath)
		if readme == "" && expLog == "" {
			// Workspace tanpa metadata file — skip biar entry DB ga kosong-melompong.
			continue
		}
		_, err := db.Exec(
			`INSERT OR IGNORE INTO workspace_meta
				(task_name, readme, experience_log)
			 VALUES (?, ?, ?)`,
			name, readme, expLog,
		)
		if err != nil {
			return fmt.Errorf("seed workspace_meta %s: %w", name, err)
		}
	}
	return nil
}

// readFileOrEmpty baca file kalau ada, return "" kalau gagal/ga ada.
// Cap 1 MiB supaya file gede ga bikin DB row besar.
func readFileOrEmpty(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if info.Size() > 1<<20 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// WorkspaceMeta adalah snapshot 1 row workspace_meta.
type WorkspaceMeta struct {
	TaskName      string `json:"task_name"`
	Readme        string `json:"readme"`
	ExperienceLog string `json:"experience_log"`
	UpdatedAt     string `json:"updated_at"`
}

// GetWorkspaceMeta ambil metadata workspace by task_name dari DB.
// Return ErrNoRows kalau task ga ada — pemanggil decide apakah fallback
// ke filesystem atau bilang ke AI bahwa kamar belum di-bootstrap.
func GetWorkspaceMeta(workspace, taskName string) (WorkspaceMeta, error) {
	var m WorkspaceMeta
	db, err := SharedSettings(workspace)
	if err != nil {
		return m, err
	}
	err = db.QueryRow(
		"SELECT task_name, readme, experience_log, updated_at FROM workspace_meta WHERE task_name = ?",
		taskName,
	).Scan(&m.TaskName, &m.Readme, &m.ExperienceLog, &m.UpdatedAt)
	return m, err
}

// ListWorkspaceMeta list semua entry workspace_meta — buat GUI tab nanti.
func ListWorkspaceMeta(workspace string) ([]WorkspaceMeta, error) {
	db, err := SharedSettings(workspace)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query("SELECT task_name, readme, experience_log, updated_at FROM workspace_meta ORDER BY task_name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkspaceMeta
	for rows.Next() {
		var m WorkspaceMeta
		if err := rows.Scan(&m.TaskName, &m.Readme, &m.ExperienceLog, &m.UpdatedAt); err != nil {
			return out, fmt.Errorf("scan workspace_meta row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iter workspace_meta rows: %w", err)
	}
	return out, nil
}
