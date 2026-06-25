package router

import (
	"strings"
	"testing"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

func ins(content, room string, imp float64) brain.InstinctDrawer {
	return brain.InstinctDrawer{Content: content, Room: room, Importance: imp}
}

// rankInstincts: yang OVERLAP relevan harus menang; yg ga relevan + importance
// rendah harus ke-skip (anti-noise); fondasi (importance tinggi) tetap kandidat.
func TestRankInstincts_RelevanceWins(t *testing.T) {
	all := []brain.InstinctDrawer{
		ins("kalau user minta kirim pesan telegram pakai tool telegram_send", "instinct_universal", 3),
		ins("strategi marketing funnel penjualan produk", "instinct_bisnis", 3),
		ins("verifikasi dulu sebelum klaim selesai anti halu", "instinct_universal", 8), // fondasi
	}
	got := rankInstincts(all, "tolong kirim pesan telegram ke owner", 3)
	if len(got) == 0 {
		t.Fatal("expected at least 1 instinct, got 0")
	}
	if !strings.Contains(got[0].Content, "telegram") {
		t.Fatalf("expected telegram instinct ranked #1, got %q", got[0].Content)
	}
}

// overlap 0 + importance < fondasi → ke-skip total (jangan inject noise).
func TestRankInstincts_AntiNoise(t *testing.T) {
	all := []brain.InstinctDrawer{
		ins("strategi marketing funnel penjualan produk", "instinct_bisnis", 3),
		ins("resep masakan rendang padang", "instinct_kehidupan", 3),
	}
	got := rankInstincts(all, "debug segfault pointer golang nil", 3)
	if len(got) != 0 {
		t.Fatalf("expected 0 (no relevance, low importance), got %d", len(got))
	}
}

// fondasi (importance >= instinctFoundationImp) tetap muncul walau overlap 0.
func TestRankInstincts_FoundationAlwaysCandidate(t *testing.T) {
	all := []brain.InstinctDrawer{
		ins("verifikasi dulu sebelum klaim anti halu cabut akar", "instinct_universal", 9),
	}
	got := rankInstincts(all, "topik benar-benar tidak nyambung xyz", 3)
	if len(got) != 1 {
		t.Fatalf("expected foundation instinct kept (1), got %d", len(got))
	}
}

// cap dihormati: max=2 → keluar max 2 walau kandidat lebih banyak.
func TestRankInstincts_RespectsCap(t *testing.T) {
	all := []brain.InstinctDrawer{
		ins("telegram kirim pesan notif", "instinct_universal", 3),
		ins("telegram broadcast pesan grup", "instinct_universal", 3),
		ins("telegram bot pesan otomatis", "instinct_universal", 3),
	}
	got := rankInstincts(all, "kirim pesan telegram", 2)
	if len(got) != 2 {
		t.Fatalf("expected cap=2, got %d", len(got))
	}
}

// buildInstinctSystem: render rapi, strip prefix room, skip content kosong.
func TestBuildInstinctSystem(t *testing.T) {
	out := buildInstinctSystem([]brain.InstinctDrawer{
		ins("pakai tool_search dulu sebelum nyerah", "instinct_universal", 5),
		ins("", "instinct_bisnis", 3), // kosong → skip
	})
	if !strings.Contains(out, "tool_search") {
		t.Fatalf("expected content rendered, got %q", out)
	}
	if !strings.Contains(out, "[universal]") {
		t.Fatalf("expected room prefix stripped to [universal], got %q", out)
	}
	if strings.Contains(out, "[bisnis]") {
		t.Fatalf("empty-content instinct should be skipped, got %q", out)
	}
}

// empty input → empty string (fails-open, ga nginject blok kosong).
func TestBuildInstinctSystem_Empty(t *testing.T) {
	if out := buildInstinctSystem(nil); out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}
