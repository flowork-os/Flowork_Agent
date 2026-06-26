// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package brain

import (
	"database/sql"
	"os"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/sidecar"

	_ "modernc.org/sqlite"
)

var pathOverrideMu sync.Mutex
var pathOverride string

func SetDBPath(p string) {
	pathOverrideMu.Lock()
	pathOverride = p
	pathOverrideMu.Unlock()
}

func DBPath() string {
	pathOverrideMu.Lock()
	o := pathOverride
	pathOverrideMu.Unlock()
	if o != "" {
		return o
	}

	return sidecar.BrainDB()
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func Available() bool {
	p := DBPath()
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

var (
	handleMu sync.Mutex
	handle   *sql.DB
	handleP  string
)

func Open() (*sql.DB, error) {
	handleMu.Lock()
	defer handleMu.Unlock()

	p := DBPath()
	if handle != nil && handleP == p {
		return handle, nil
	}
	if handle != nil {
		_ = handle.Close()
		handle = nil
	}

	dsn := "file:" + p + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(2)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	handle = db
	handleP = p
	return handle, nil
}
