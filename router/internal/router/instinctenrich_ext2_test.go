package router

import (
	"context"
	"testing"

	"github.com/flowork-os/flowork_Router/internal/brain"
)

// roomsOf — kumpulin Room dari hasil selektor (buat assertion).
func roomsOf(ds []brain.InstinctDrawer) map[string]bool {
	out := map[string]bool{}
	for _, d := range ds {
		out[d.Room] = true
	}
	return out
}

func scopedFixture() []brain.InstinctDrawer {
	// importance tinggi (>= foundation) → semua jadi kandidat di rankInstincts walau overlap 0.
	return []brain.InstinctDrawer{
		ins("verifikasi anti halu", "instinct_universal", 9),
		ins("pakai tool sebelum nyerah", "instinct_tool", 9),
		ins("audit pointer nil golang", "instinct_coding", 9),
		ins("funnel marketing penjualan", "instinct_bisnis", 9),
		ins("hardening sql injection", "instinct_security", 9),
	}
}

// SCOPED ON + agent ke-map (coding) → cuma baseline(universal/tool)+coding lolos; bisnis/security DROP.
func TestScoped_FiltersByRole(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0") // jalur deterministik rankInstincts (no brain.Open)
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "1")
	t.Setenv("FLOWORK_INSTINCT_SCOPE_MAP", "test-coder:instinct_coding")
	ctx := WithAgentID(context.Background(), "test-coder")
	got := scopedInstinctSelector(ctx, scopedFixture(), "apa aja", 10)
	rooms := roomsOf(got)
	for _, want := range []string{"instinct_universal", "instinct_tool", "instinct_coding"} {
		if !rooms[want] {
			t.Errorf("expected %s kept, missing. got rooms=%v", want, rooms)
		}
	}
	for _, no := range []string{"instinct_bisnis", "instinct_security"} {
		if rooms[no] {
			t.Errorf("expected %s DROPPED (out of role), but present. got rooms=%v", no, rooms)
		}
	}
}

// SCOPED OFF → fails-open: SEMUA domain eligible (bisnis & security ikut).
func TestScoped_OffIsFailOpen(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "0")
	ctx := WithAgentID(context.Background(), "test-coder")
	rooms := roomsOf(scopedInstinctSelector(ctx, scopedFixture(), "apa aja", 10))
	if !rooms["instinct_bisnis"] || !rooms["instinct_security"] {
		t.Errorf("switch off must be fails-open (all domains), got rooms=%v", rooms)
	}
}

// SCOPED ON tapi agent id KOSONG (external/belum-rebuild) → fails-open (semua domain).
func TestScoped_EmptyAgentFailOpen(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "1")
	rooms := roomsOf(scopedInstinctSelector(context.Background(), scopedFixture(), "apa aja", 10))
	if !rooms["instinct_bisnis"] || !rooms["instinct_security"] {
		t.Errorf("empty agent must be fails-open, got rooms=%v", rooms)
	}
}

// SCOPED ON tapi agent TIDAK ke-map → fails-open (belum di-scope = perilaku lama).
func TestScoped_UnmappedFailOpen(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "1")
	t.Setenv("FLOWORK_INSTINCT_SCOPE_MAP", "")
	ctx := WithAgentID(context.Background(), "agent-tak-dikenal")
	rooms := roomsOf(scopedInstinctSelector(ctx, scopedFixture(), "apa aja", 10))
	if !rooms["instinct_bisnis"] || !rooms["instinct_security"] {
		t.Errorf("unmapped agent must be fails-open, got rooms=%v", rooms)
	}
}

// baseline (universal+tool) SELALU lolos walau role cuma bisnis → anti-starvation.
func TestScoped_BaselineAlwaysKept(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "1")
	t.Setenv("FLOWORK_INSTINCT_SCOPE_MAP", "biz-bot:instinct_bisnis")
	ctx := WithAgentID(context.Background(), "biz-bot")
	rooms := roomsOf(scopedInstinctSelector(ctx, scopedFixture(), "apa aja", 10))
	if !rooms["instinct_universal"] || !rooms["instinct_tool"] {
		t.Errorf("baseline universal+tool must always pass, got rooms=%v", rooms)
	}
	if rooms["instinct_coding"] || rooms["instinct_security"] {
		t.Errorf("non-role domains must drop, got rooms=%v", rooms)
	}
}
