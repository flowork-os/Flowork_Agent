package mesh

import (
	"database/sql"
	"testing"
)

func TestRegisterMeshFilter(t *testing.T) {
	saved := extraMeshFilters
	extraMeshFilters = nil
	defer func() { extraMeshFilters = saved }()

	called := false
	RegisterMeshFilter(MeshFilter{
		Name: "t1",
		Run:  func(db *sql.DB, pkt Packet, c string) FilterDecision { called = true; return FilterDecision{Layer: "L10-t1", Decision: "pass"} },
	})
	RegisterMeshFilter(MeshFilter{Name: "nil"}) // Run==nil → diabaikan
	out, rej := runExtraMeshFilters(nil, Packet{}, "x")
	if !called || len(out) != 1 || rej {
		t.Fatalf("mau called && 1 decision && !reject; dapat called=%v len=%d rej=%v", called, len(out), rej)
	}
}

func TestMeshFilterSwitchAndReject(t *testing.T) {
	saved := extraMeshFilters
	extraMeshFilters = nil
	defer func() { extraMeshFilters = saved }()

	RegisterMeshFilter(MeshFilter{
		Name: "blocker", Switch: "FLOWORK_TEST_MESH_X",
		Run: func(db *sql.DB, pkt Packet, c string) FilterDecision { return FilterDecision{Layer: "L10", Decision: "reject", Reason: "test"} },
	})
	t.Setenv("FLOWORK_TEST_MESH_X", "0")
	if out, rej := runExtraMeshFilters(nil, Packet{}, "x"); len(out) != 0 || rej {
		t.Fatalf("switch off → skip; dapat len=%d rej=%v", len(out), rej)
	}
	t.Setenv("FLOWORK_TEST_MESH_X", "1")
	if out, rej := runExtraMeshFilters(nil, Packet{}, "x"); len(out) != 1 || !rej {
		t.Fatalf("switch on → reject; dapat len=%d rej=%v", len(out), rej)
	}
}

func TestMeshFilterFailsOpen(t *testing.T) {
	saved := extraMeshFilters
	extraMeshFilters = nil
	defer func() { extraMeshFilters = saved }()
	RegisterMeshFilter(MeshFilter{Name: "boom", Run: func(db *sql.DB, pkt Packet, c string) FilterDecision { panic("x") }})
	RegisterMeshFilter(MeshFilter{Name: "ok", Run: func(db *sql.DB, pkt Packet, c string) FilterDecision { return FilterDecision{Layer: "L10", Decision: "pass"} }})
	out, rej := runExtraMeshFilters(nil, Packet{}, "x")
	if len(out) != 2 || rej {
		t.Fatalf("fails-open: panic recovered jadi pass; dapat len=%d rej=%v", len(out), rej)
	}
}
