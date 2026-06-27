// deathletter_ext_test.go — bukti F3 (Death-Letter): catat kematian → ke-list balik.
package main

import (
	"path/filepath"
	"testing"
)

func TestDeathLetterRoundTrip(t *testing.T) {
	// arahin store ke temp (FLOWORK_AGENTS_DIR → parent jadi temp) — ga ngotori ~/.flowork.
	t.Setenv("FLOWORK_AGENTS_DIR", filepath.Join(t.TempDir(), "agents"))

	if got := listDeathLetters(); len(got) != 0 {
		t.Fatalf("awal harusnya kosong, dapat %d", len(got))
	}
	recordDeathLetter("pantun-pack", "category", "Generator Pantun",
		"reaped (owner-approved: low-karma/broken)", []string{"pantun-worker", "pantun-synth"})

	got := listDeathLetters()
	if len(got) != 1 {
		t.Fatalf("harusnya 1 surat, dapat %d", len(got))
	}
	d := got[0]
	if d.ID != "pantun-pack" || d.Name != "Generator Pantun" || d.Reason == "" || d.At == "" {
		t.Errorf("surat ga lengkap: %+v", d)
	}
	if len(d.Agents) != 2 {
		t.Errorf("agents harusnya 2, dapat %v", d.Agents)
	}

	// surat ke-2 → terbaru di depan (prepend).
	recordDeathLetter("resep-pack", "category", "Resep Masak", "manual uninstall (owner)", nil)
	got = listDeathLetters()
	if len(got) != 2 || got[0].ID != "resep-pack" {
		t.Fatalf("terbaru harusnya di depan, dapat %+v", got)
	}
}
