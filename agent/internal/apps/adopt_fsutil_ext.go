// adopt_fsutil_ext.go — SIBLING package apps (NON-frozen, seam). Util fs/json buat AdoptRepo.
// Dipisah biar adopt_ext.go fokus orchestration. Dipakai cuma jalur adopt (non-frozen).
package apps

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

// trimTail — potong string ke n char terakhir (buat ringkas log error).
func trimTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// writeJSONFile — tulis v sbg JSON indented ke path.
func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// copyTree — copy folder src → dst rekursif. Skip .git (repo bersih, hemat ruang). Anti-symlink-escape:
// symlink di-skip (ga di-follow) biar copy ga keluar ke luar src.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		// skip .git dan isinya
		if info.IsDir() && (info.Name() == ".git") {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			return nil // skip symlink (jangan follow keluar src)
		default:
			return copyFile(p, target, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
