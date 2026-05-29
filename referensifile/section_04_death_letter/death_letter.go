package db

// death_letter.go — helper untuk Death Letter & Reinkarnasi (roadmap_ai_external.md
// ekspansi visi #3).
//
// Konsep: warga AI yang retire/upgrade WAJIB tulis death-letter berisi
// rangkuman gotchas + filosofi yang dipelajari. Saat warga baru spawn ke
// workspace yg sama, ingatan pendahulu auto-inject ke system context.
// Raga berganti, SOUL berinkarnasi.
//
// Per Ayah 2026-04-25 (arsitektur baru): semua doktrin di DB. Migrate
// dari state/death-letters/ filesystem ke tabel ini untuk query cepat
// + auto-inject. File .md tetap ada (transisional, hapus 1 bulan stabil).

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DeathLetter adalah snapshot 1 row death_letters.
type DeathLetter struct {
	ID        int64  `json:"id"`
	AgentID   string `json:"agent_id"`
	Workspace string `json:"workspace"`
	Content   string `json:"content"`
	Reason    string `json:"reason"`
	WrittenAt string `json:"written_at"`
}

// WriteDeathLetter tulis wasiat warga retiring ke DB. Append-only —
// satu warga bisa tulis multiple letter (handoff bertahap, atau
// retire bertingkat).
func WriteDeathLetter(workspace, agentID, taskWorkspace, content, reason string) (int64, error) {
	if agentID == "" {
		return 0, fmt.Errorf("death_letter: agent_id wajib")
	}
	if content == "" {
		return 0, fmt.Errorf("death_letter: content wajib")
	}
	db, err := SharedSettings(workspace)
	if err != nil {
		return 0, err
	}
	res, err := db.Exec(
		`INSERT INTO death_letters (agent_id, workspace, content, reason)
		 VALUES (?, ?, ?, ?)`,
		agentID, taskWorkspace, content, reason,
	)
	if err != nil {
		return 0, fmt.Errorf("write death_letter: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("write death_letter last insert id: %w", err)
	}
	return id, nil
}

// ReadLatestDeathLetter — wasiat terbaru di workspace tertentu (untuk
// auto-inject saat warga baru spawn ke kamar yang sama).
//
// Empty taskWorkspace → ambil yang terbaru lintas-workspace.
func ReadLatestDeathLetter(workspace, taskWorkspace string) (DeathLetter, error) {
	var dl DeathLetter
	db, err := SharedSettings(workspace)
	if err != nil {
		return dl, err
	}
	q := `SELECT id, agent_id, workspace, content, reason, written_at
	      FROM death_letters`
	args := []any{}
	if taskWorkspace != "" {
		q += ` WHERE workspace = ?`
		args = append(args, taskWorkspace)
	}
	q += ` ORDER BY written_at DESC LIMIT 1`
	err = db.QueryRow(q, args...).Scan(&dl.ID, &dl.AgentID, &dl.Workspace, &dl.Content, &dl.Reason, &dl.WrittenAt)
	if err == sql.ErrNoRows {
		return dl, sql.ErrNoRows
	}
	return dl, err
}

// MigrateDeathLettersFromFilesystem scan state/death-letters/*.md dan insert
// ke tabel death_letters. Idempotent — pakai (agent_id, written_at) hash
// sederhana untuk skip kalau udah ke-migrate.
//
// Filename convention:
//   - "<agent>.md"
//   - "<YYYY-MM-DD>-<agent>.md"
//   - "<YYYY-MM-DDTHH-MM-SS>-<agent>.md"  (Z atau tidak)
//   - "<YYYYMMDD-HHMMSS>-<agent>.md"
//
// Agent_id di-extract via regex strip timestamp prefix; kalau gagal parse,
// pakai full basename (sans .md). Reason: filesystem migration label.
//
// Per Ayah 2026-04-25 prio rc144: migrate sweep state/death-letters/ → DB.
// File .md tetep ada (transisional, hapus 1 bulan setelah stabil).
func MigrateDeathLettersFromFilesystem(db *sql.DB, workspaceRoot string) (int, error) {
	dlDir := filepath.Join(workspaceRoot, "state", "death-letters")
	entries, err := os.ReadDir(dlDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // Folder belum ada — fresh install, OK.
		}
		return 0, fmt.Errorf("read death-letters dir: %w", err)
	}

	// Regex: capture timestamp prefix kalau ada.
	// Match: 2026-04-25-merpati.md  → group=merpati
	// Match: 2026-04-25T12-30-45Z-merpati.md → group=merpati
	// Match: 20260425-123045-merpati.md → group=merpati
	tsPrefixRe := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}(?:T\d{2}-\d{2}-\d{2}Z?)?|\d{8}-\d{6})[-_](.+)$`)

	migrated := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}

		path := filepath.Join(dlDir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > 5<<20 { // 5 MiB cap
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Parse agent_id dari filename.
		base := strings.TrimSuffix(name, filepath.Ext(name))
		agentID := base
		if m := tsPrefixRe.FindStringSubmatch(base); len(m) >= 3 {
			agentID = m[2]
		}

		// Idempotent check: sudah ada entry dengan agent_id + content size sama?
		// Pakai content length sebagai proxy hash (cheap).
		var existingID int64
		err = db.QueryRow(
			`SELECT id FROM death_letters WHERE agent_id = ? AND length(content) = ? LIMIT 1`,
			agentID, len(data),
		).Scan(&existingID)
		if err == nil {
			continue // sudah ke-migrate sebelumnya
		}
		if err != sql.ErrNoRows {
			return migrated, fmt.Errorf("check existing %s: %w", agentID, err)
		}

		// Insert dengan reason="filesystem_migrate" untuk audit trail.
		_, err = db.Exec(
			`INSERT INTO death_letters (agent_id, workspace, content, reason)
			 VALUES (?, ?, ?, ?)`,
			agentID, "", string(data), "filesystem_migrate:"+name,
		)
		if err != nil {
			return migrated, fmt.Errorf("insert death_letter %s: %w", name, err)
		}
		migrated++
	}
	return migrated, nil
}

// HasPreRetireDeathLetter — B-9 fix: cek apakah agent sudah tulis death-letter
// sebelum di-retire. Caller (GUI agent_retire handler) wajib panggil ini
// sebelum flip status='retired' dan auto-fallback. Kalau warga MANUAL nulis
// (via tool death_letter_write), entry sudah ada → skip auto-fallback.
//
// Return (true, written_at) kalau sudah ada wasiat manual. (false, "") kalau
// belum (caller harus prompt warga write_letter dulu, atau auto-write minimal).
func HasPreRetireDeathLetter(workspace, agentID string) (bool, string, error) {
	if agentID == "" {
		return false, "", fmt.Errorf("death_letter: agent_id wajib")
	}
	db, err := SharedSettings(workspace)
	if err != nil {
		return false, "", err
	}
	var writtenAt string
	err = db.QueryRow(
		`SELECT written_at FROM death_letters
		 WHERE agent_id = ? AND reason NOT LIKE 'filesystem_migrate%' AND reason NOT LIKE 'gui:agent_retire%'
		 ORDER BY written_at DESC LIMIT 1`,
		agentID,
	).Scan(&writtenAt)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("check pre-retire death_letter: %w", err)
	}
	return true, writtenAt, nil
}

// ListDeathLetters — list n entry terbaru. Optional filter by taskWorkspace.
// Buat GUI tab + tool death_letter_read.
func ListDeathLetters(workspace, taskWorkspace string, n int) ([]DeathLetter, error) {
	if n <= 0 {
		n = 10
	}
	db, err := SharedSettings(workspace)
	if err != nil {
		return nil, err
	}
	q := `SELECT id, agent_id, workspace, content, reason, written_at
	      FROM death_letters`
	args := []any{}
	if taskWorkspace != "" {
		q += ` WHERE workspace = ?`
		args = append(args, taskWorkspace)
	}
	q += ` ORDER BY written_at DESC LIMIT ?`
	args = append(args, n)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeathLetter
	for rows.Next() {
		var dl DeathLetter
		if err := rows.Scan(&dl.ID, &dl.AgentID, &dl.Workspace, &dl.Content, &dl.Reason, &dl.WrittenAt); err != nil {
			return out, fmt.Errorf("scan death_letter row: %w", err)
		}
		out = append(out, dl)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iter death_letter rows: %w", err)
	}
	return out, nil
}
