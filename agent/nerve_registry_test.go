package main

import "testing"

// Seed (nerve_seed_ext.go init) jalan sebelum test → papan udah keisi katalog peta-saraf.
func TestNerve_SeedLoaded(t *testing.T) {
	if c := NerveCount(); c < 100 {
		t.Fatalf("papan saraf harusnya keisi katalog peta-saraf (>=100), dapat %d", c)
	}
	// switch nyata harus ada + kind bener.
	if n, ok := NerveByName("FLOWORK_EDITION"); !ok || n.Kind != "switch" {
		t.Fatalf("FLOWORK_EDITION harus terdaftar sbg switch, dapat ok=%v kind=%q", ok, n.Kind)
	}
	// registry nyata harus ada.
	if n, ok := NerveByName("RegisterDetector"); !ok || n.Kind != "registry" {
		t.Fatalf("RegisterDetector harus terdaftar sbg registry, dapat ok=%v kind=%q", ok, n.Kind)
	}
	// di luar daftar → ga punya (F2 bukti lulus: usul di luar daftar ketauan).
	if _, ok := NerveByName("FLOWORK_TIDAK_ADA_XYZ"); ok {
		t.Fatal("saraf ngaco harusnya TIDAK terdaftar")
	}
}

func TestNerve_FailSafe(t *testing.T) {
	before := NerveCount()
	RegisterNerve(Nerve{Name: "", Kind: "switch"})            // nama kosong → diabaikan
	RegisterNerve(Nerve{Name: "FLOWORK_BAD", Kind: "ngaco"})  // kind ga sah → diabaikan
	if NerveCount() != before {
		t.Fatalf("saraf invalid harus diabaikan (papan ga korup), before=%d after=%d", before, NerveCount())
	}
}

func TestNerve_Idempotent(t *testing.T) {
	RegisterNerve(Nerve{Name: "FLOWORK_DUMMY_TEST", Kind: "switch", Default: "a", Desc: "x"})
	c1 := NerveCount()
	RegisterNerve(Nerve{Name: "FLOWORK_DUMMY_TEST", Kind: "switch", Default: "b", Desc: "y"}) // sama → overwrite
	if NerveCount() != c1 {
		t.Fatalf("re-register nama sama harus overwrite (bukan dobel), c1=%d c2=%d", c1, NerveCount())
	}
	if n, _ := NerveByName("FLOWORK_DUMMY_TEST"); n.Default != "b" {
		t.Fatalf("overwrite harus update nilai, dapat default=%q", n.Default)
	}
}

func TestNerve_Channels(t *testing.T) {
	if ch := NerveChannels(); len(ch) != 3 {
		t.Fatalf("harus 3 saluran sah, dapat %v", ch)
	}
	// F3 acuan: switch/data/modul sah; "edit kode inti" HARAM.
	for _, ok := range []string{"switch", "data", "modul"} {
		if !NerveChannelValid(ok) {
			t.Fatalf("%q harusnya saluran sah", ok)
		}
	}
	for _, bad := range []string{"edit-core", "main.go", ""} {
		if NerveChannelValid(bad) {
			t.Fatalf("%q HARUS ditolak (di luar 3 saluran)", bad)
		}
	}
}
