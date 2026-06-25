package brain

import (
	"path/filepath"
	"testing"
)

// TestSeedInstincts — fresh brain → ke-seed insting basic + persona, idempotent, NOL personal-
// pihak-ketiga (history Aola/teman). Nama pencipta (Aola) di persona = branding, BOLEH.
func TestSeedInstincts(t *testing.T) {
	SetDBPath(filepath.Join(t.TempDir(), "brain.sqlite"))
	n, err := SeedInstincts()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n < 200 {
		t.Fatalf("expected ~282 insting ke-seed, dapat %d", n)
	}
	if n2, err := SeedInstincts(); err != nil || n2 != 0 {
		t.Fatalf("idempotent gagal: n2=%d err=%v", n2, err)
	}
	db, err := OpenRW()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// privasi drawers: cuma instinct_*, NOL marker training/teman
	var bad int
	_ = db.QueryRow(`SELECT COUNT(*) FROM drawers WHERE content LIKE '%TRAINING OWNER%' OR content LIKE '%guru gitar%' OR room NOT LIKE 'instinct\_%' ESCAPE '\'`).Scan(&bad)
	if bad != 0 {
		t.Fatalf("PRIVASI/scope bocor (drawers): %d", bad)
	}
	// persona ke-seed + NOL third-party
	var np, badp int
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates`).Scan(&np)
	if np < 1 {
		t.Fatalf("persona ga ke-seed: %d", np)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM prompt_templates WHERE content LIKE '%TRAINING OWNER%' OR content LIKE '%guru gitar%'`).Scan(&badp)
	if badp != 0 {
		t.Fatalf("PRIVASI persona bocor (third-party): %d", badp)
	}
}
