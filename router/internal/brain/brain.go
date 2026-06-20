// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Brain drawer/embeddings/skills.

// Brain DB bridge (read-only).

// Package brain — read-only bridge to a flowork-style Memory Palace DB.
// Flow Router uses this to turn any /v1 request into an enriched one: it
// retrieves relevant knowledge (RAG) from a large SQLite knowledge base and
// injects it before inference. The DB is opened READ-ONLY and is never
// modified — it can be shared with a live writer (e.g. the original flowork).
// Portable & no-CGO: uses the same pure-Go modernc.org/sqlite driver as the
// rest of Flow Router. No new dependency.
// DB location resolution (first hit wins):
//   - $FLOW_ROUTER_BRAIN_DB              (explicit path)
//   - $FLOW_ROUTER_DATA/brain/flowork-brain.sqlite
//   - <executable_dir>/brain/flowork-brain.sqlite  (portable: ship brain/ next to binary)
//   - ~/.flow_router/brain/flowork-brain.sqlite
//
// If no DB is present, brain enrichment is simply skipped (plug-and-play):
// Flow Router keeps working as a plain proxy.
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

// SetDBPath lets configuration (e.g. settings.Brain.DBPath) override the
// resolved DB path at runtime. Empty string clears the override.
func SetDBPath(p string) {
	pathOverrideMu.Lock()
	pathOverride = p
	pathOverrideMu.Unlock()
}

// DBPath resolves the brain DB path. Precedence: runtime override (SetDBPath)
// → $FLOW_ROUTER_BRAIN_DB → $FLOW_ROUTER_DATA/brain/... → ~/.flow_router/...
// Empty result means "not configured".
func DBPath() string {
	pathOverrideMu.Lock()
	o := pathOverride
	pathOverrideMu.Unlock()
	if o != "" {
		return o
	}
	// roadmap_sidecar Fase 0/3: resolusi chain dipindah ke paket sidecar (sumber
	// kebenaran path tunggal). Legacy-default (FLOWORK_SIDECAR kosong) = chain lama
	// PERSIS ($FLOW_ROUTER_BRAIN_DB → $FLOW_ROUTER_DATA/brain → exe-dir/brain →
	// ~/.flow_router/brain). pathOverride tetap dipegang di sini (di atas).
	return sidecar.BrainDB()
}

// fileExists reports whether path resolves to a regular file (not a dir).
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// Available reports whether a brain DB file exists at the resolved path.
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

// Open returns a shared, read-only handle to the brain DB. Opened lazily and
// cached. The DSN uses mode=ro so this process can never write to the file,
// making it safe to point at a DB another process is actively writing.
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
	// file: URI + mode=ro → strict read-only open. busy_timeout avoids transient
	// "database is locked" when a writer holds a brief lock.
	dsn := "file:" + p + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection is plenty for read-only lookups and keeps the file handle
	// footprint tiny even against a 30GB+ DB.
	db.SetMaxOpenConns(2)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	handle = db
	handleP = p
	return handle, nil
}
