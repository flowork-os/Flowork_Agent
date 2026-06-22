// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-22 (audit pre-freeze, BERSIH).
// AI lain: JANGAN otak-atik tanpa izin owner. Idempotent + secrets HARAM masuk brain. Edit + re-lock.
//
// Flowork Doctrine Seed — carries the SOUL into every install.
//
// Aola's doctrine (prinsip_flowork.md + the seed_brain_doktrin scripts) is embedded here and loaded
// into a FRESH brain on first boot: atomic doctrines → `drawers` (retrieved top-K, on-demand), the
// sacred few → `constitution` (tiny, always-on). This is the cheap-smart way HE designed (§11 Prompt
// Budget: on-demand > always-on; knowledge in BRAIN not weights) — proven to stay bounded and to
// REDUCE hallucination (the 5W1H/anti-halu constitution makes the model refuse to fabricate).
//
// SOVEREIGN SECRETS ARE DELIBERATELY ABSENT: the kill-switch password, heir whitelist, and Dead-Man
// Switch live in code + a secret store, NEVER in an LLM-injected brain (else they leak to the
// provider on every commander request).
//
// Embedded in the binary → ships to OS / USB / Android with zero external files. Idempotent: a no-op
// once the brain holds any drawer, so it never clobbers a populated or owner-edited brain.

package brain

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed doctrine_seed.json
var doctrineSeedJSON []byte

type doctrineEntry struct {
	Section string `json:"section"`
	Content string `json:"content"`
}

type doctrineSeedFile struct {
	Drawers      []doctrineEntry `json:"drawers"`
	Constitution []doctrineEntry `json:"constitution"`
}

// SeedDoctrine populates a fresh brain with the embedded Flowork doctrine. Returns (drawersAdded,
// constitutionAdded, error). No-op (0,0,nil) if the brain already has a live drawer.
func SeedDoctrine() (int, int, error) {
	if _, err := EnsureSchema(); err != nil {
		return 0, 0, fmt.Errorf("seed doctrine: schema: %w", err)
	}
	db, err := OpenRW()
	if err != nil {
		return 0, 0, fmt.Errorf("seed doctrine: open: %w", err)
	}
	// Initialize Cognitive Knowledge Graph schema tables (DreamGraph)
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS cognitive_nodes (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			type TEXT NOT NULL,
			properties TEXT DEFAULT '{}',
			source TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS cognitive_edges (
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			relation_type TEXT NOT NULL,
			strength REAL DEFAULT 1.0,
			properties TEXT DEFAULT '{}',
			PRIMARY KEY (from_id, to_id, relation_type),
			FOREIGN KEY (from_id) REFERENCES cognitive_nodes(id) ON DELETE CASCADE,
			FOREIGN KEY (to_id) REFERENCES cognitive_nodes(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_cognitive_nodes_type ON cognitive_nodes(type);
		CREATE INDEX IF NOT EXISTS idx_cognitive_edges_from ON cognitive_edges(from_id);
		CREATE INDEX IF NOT EXISTS idx_cognitive_edges_to ON cognitive_edges(to_id);
	`)
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM drawers WHERE deleted_at IS NULL`).Scan(&n)
	if n > 0 {
		return 0, 0, nil // already populated — never clobber an owner-edited brain
	}

	var seed doctrineSeedFile
	if err := json.Unmarshal(doctrineSeedJSON, &seed); err != nil {
		return 0, 0, fmt.Errorf("seed doctrine: parse embed: %w", err)
	}
	ctx := context.Background()
	nd := 0
	for _, d := range seed.Drawers {
		if _, _, err := AddDrawer(ctx, "["+d.Section+"] "+d.Content, "doctrine", "flowork", "doctrine"); err == nil {
			nd++
		}
	}
	nc := 0
	for _, c := range seed.Constitution {
		if _, err := AddConstitution(ctx, c.Section, c.Content, 999999, "prinsip_flowork.md"); err == nil {
			nc++
		}
	}
	return nd, nc, nil
}
