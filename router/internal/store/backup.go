// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultKeepBackups = 5

func sanitizeBackupLabel(label string) string {
	var b strings.Builder
	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "manual"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func BackupsDir() string {
	return filepath.Join(dataDir(), "backups")
}

type BackupInfo struct {
	Label     string    `json:"label"`
	Dir       string    `json:"dir"`
	DBPath    string    `json:"dbPath"`
	CreatedAt time.Time `json:"createdAt"`
	SizeBytes int64     `json:"sizeBytes"`
}

func Backup(label string, keepN int) (*BackupInfo, error) {
	d, _ := Open()
	return backupWithConn(d, label, keepN)
}

func backupWithConn(d *sql.DB, label string, keepN int) (*BackupInfo, error) {
	label = sanitizeBackupLabel(label)
	if keepN <= 0 {
		keepN = defaultKeepBackups
	}
	root := BackupsDir()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir backups: %w", err)
	}
	slug := fmt.Sprintf("%s-%s", label, time.Now().UTC().Format("20060102T150405Z"))
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir snap: %w", err)
	}
	dest := filepath.Join(dir, "data.sqlite")

	if err := snapshotIntoWithConn(d, dest); err != nil {

		_ = os.RemoveAll(dir)
		return nil, err
	}
	info := &BackupInfo{Label: label, Dir: dir, DBPath: dest, CreatedAt: time.Now().UTC()}
	if st, err := os.Stat(dest); err == nil {
		info.SizeBytes = st.Size()
	}
	_ = pruneBackups(root, keepN)
	return info, nil
}

func ListBackups() ([]BackupInfo, error) {
	root := BackupsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackupInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(root, e.Name())
		dbp := filepath.Join(full, "data.sqlite")
		st, err := os.Stat(dbp)
		if err != nil {
			continue
		}
		info, _ := e.Info()
		ct := st.ModTime().UTC()
		if info != nil {
			ct = info.ModTime().UTC()
		}
		out = append(out, BackupInfo{
			Label:     e.Name(),
			Dir:       full,
			DBPath:    dbp,
			CreatedAt: ct,
			SizeBytes: st.Size(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func snapshotIntoWithConn(d *sql.DB, dest string) error {
	if d != nil {

		if _, err := d.Exec(`VACUUM INTO ?`, dest); err == nil {
			return nil
		}

	}
	src := DBPath()
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

func pruneBackups(root string, keepN int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type item struct {
		name  string
		mtime time.Time
	}
	var dirs []item
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, item{name: e.Name(), mtime: info.ModTime()})
	}
	if len(dirs) <= keepN {
		return nil
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].mtime.After(dirs[j].mtime) })
	for _, d := range dirs[keepN:] {
		_ = os.RemoveAll(filepath.Join(root, d.name))
	}
	return nil
}
