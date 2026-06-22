package agentdb

import (
	"testing"
	"time"
)

func TestColdArchive(t *testing.T) {
	s := openTestStore(t)
	old := time.Now().UTC().AddDate(0, 0, -200).Format(time.RFC3339)
	recent := time.Now().UTC().Format(time.RFC3339)

	mk := func(id, typ string, hit int64, seen string) {
		if _, err := s.UpsertNode(CogNode{ID: id, Label: id, Type: typ, Status: "active", Confidence: 0.5}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE cognitive_nodes SET hit_count=?, last_seen_at=? WHERE id=?`, hit, seen, id); err != nil {
			t.Fatal(err)
		}
	}
	mk("old-mem", "memory", 1, old)        // ARCHIVE: tua+low-hit+bulk
	mk("old-person", "person", 1, old)     // KEEP: tipe identitas
	mk("old-instinct", "instinct", 1, old) // KEEP: tipe instinct (inti)
	mk("hot-mem", "memory", 5, old)        // KEEP: hit tinggi
	mk("new-mem", "memory", 1, recent)     // KEEP: masih baru

	// GATE: ambang tinggi (1000) → no-op walau ada yg eligible.
	if n, err := s.ArchiveColdNodes(90, 1, 1000); err != nil || n != 0 {
		t.Fatalf("gate gagal: n=%d err=%v (harus 0, di bawah ambang)", n, err)
	}

	// Ambang 0 → aktif. Cuma old-mem yg ke-archive.
	n, err := s.ArchiveColdNodes(90, 1, 0)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 1 {
		t.Fatalf("archived=%d want 1 (cuma old-mem)", n)
	}
	chk := func(id, want string) {
		var st string
		if err := s.db.QueryRow(`SELECT status FROM cognitive_nodes WHERE id=?`, id).Scan(&st); err != nil {
			t.Fatal(err)
		}
		if st != want {
			t.Errorf("%s status=%s want %s", id, st, want)
		}
	}
	chk("old-mem", "archived")
	chk("old-person", "active")
	chk("old-instinct", "active")
	chk("hot-mem", "active")
	chk("new-mem", "active")

	// Recall HARUS skip archived (SearchNodesByLabel filter status='active').
	for _, h := range s.SearchNodesByLabel("old-mem", 5) {
		if h.ID == "old-mem" {
			t.Fatal("archived node BOCOR ke recall (harus ke-skip)")
		}
	}

	// Restore → balik active + ke-recall lagi.
	if err := s.RestoreArchivedNode("old-mem"); err != nil {
		t.Fatal(err)
	}
	chk("old-mem", "active")

	if ca, _ := s.CountArchivedNodes(); ca != 0 {
		t.Fatalf("setelah restore archived count=%d want 0", ca)
	}
}
