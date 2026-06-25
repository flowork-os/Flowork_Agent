package router

import (
	"context"
	"testing"
)

// #11 brain-as-service. EXTERNAL caller (agent-id kosong) + switch ON →
// instinct_tool DROP (anti-halu), universal + reasoning per-domain TETAP lolos.
func TestExternalScope_DropsToolInstinct(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0") // jalur deterministik (no brain.Open)
	t.Setenv("FLOWORK_BRAIN_EXTERNAL_SCOPE", "1")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "0") // independen dari master per-agent switch
	rooms := roomsOf(scopedInstinctSelector(context.Background(), scopedFixture(), "apa aja", 10))
	if rooms["instinct_tool"] {
		t.Errorf("external harus DROP instinct_tool (anti-halu), got rooms=%v", rooms)
	}
	for _, want := range []string{"instinct_universal", "instinct_coding", "instinct_security", "instinct_bisnis"} {
		if !rooms[want] {
			t.Errorf("external harus tetep kasih reasoning %s, missing. got rooms=%v", want, rooms)
		}
	}
}

// EXTERNAL switch ON tapi caller PUNYA agent-id (internal) → external-scope TIDAK kena;
// instinct_tool tetep lolos (perilaku internal normal).
func TestExternalScope_InternalUnaffected(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0")
	t.Setenv("FLOWORK_BRAIN_EXTERNAL_SCOPE", "1")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "0") // master off → internal fails-open (semua)
	ctx := WithAgentID(context.Background(), "some-internal-agent")
	rooms := roomsOf(scopedInstinctSelector(ctx, scopedFixture(), "apa aja", 10))
	if !rooms["instinct_tool"] {
		t.Errorf("internal (punya agent-id) ga boleh kena external-scope, instinct_tool harus ada. got=%v", rooms)
	}
}

// EXTERNAL switch OFF (default) + agent-id kosong → fails-open PENUH (instinct_tool ikut),
// jaga agent template-lama (belum rebuild, id kosong) yg BUTUH instinct_tool ga ke-starve.
func TestExternalScope_OffKeepsToolForLegacy(t *testing.T) {
	t.Setenv("FLOWORK_INSTINCT_SEMANTIC", "0")
	t.Setenv("FLOWORK_BRAIN_EXTERNAL_SCOPE", "0")
	t.Setenv("FLOWORK_INSTINCT_SCOPED", "1")
	rooms := roomsOf(scopedInstinctSelector(context.Background(), scopedFixture(), "apa aja", 10))
	if !rooms["instinct_tool"] {
		t.Errorf("switch off harus fails-open (instinct_tool ikut buat agent lama), got=%v", rooms)
	}
}
