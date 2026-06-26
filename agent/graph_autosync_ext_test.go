package main

import (
	"context"
	"testing"

	"flowork-gui/internal/agentdb"
)

// TestRegisterGraphProjection — seam ke-trigger + nil-Run aman + total ke-akumulasi.
func TestRegisterGraphProjection(t *testing.T) {
	saved := extraGraphProjections
	extraGraphProjections = nil
	defer func() { extraGraphProjections = saved }()

	called := false
	RegisterGraphProjection(GraphProjection{
		Name: "test-proj",
		Run: func(ctx context.Context, store *agentdb.Store, scope string) (int, error) {
			called = true
			return 3, nil
		},
	})
	RegisterGraphProjection(GraphProjection{Name: "nil-run"}) // Run==nil → diabaikan

	n := runExtraGraphProjections(context.Background(), nil, "agent:test")
	if !called {
		t.Fatal("proyeksi terdaftar tidak dipanggil")
	}
	if n != 3 {
		t.Fatalf("total: mau 3, dapat %d", n)
	}
}

// TestGraphProjectionSwitchGate — switch OFF skip, ON jalan; switch kosong selalu jalan.
func TestGraphProjectionSwitchGate(t *testing.T) {
	saved := extraGraphProjections
	extraGraphProjections = nil
	defer func() { extraGraphProjections = saved }()

	RegisterGraphProjection(GraphProjection{
		Name:   "gated",
		Switch: "FLOWORK_TEST_PROJ_X",
		Run:    func(ctx context.Context, store *agentdb.Store, scope string) (int, error) { return 5, nil },
	})
	t.Setenv("FLOWORK_TEST_PROJ_X", "0")
	if n := runExtraGraphProjections(context.Background(), nil, "s"); n != 0 {
		t.Fatalf("switch off harus skip, dapat %d", n)
	}
	t.Setenv("FLOWORK_TEST_PROJ_X", "1")
	if n := runExtraGraphProjections(context.Background(), nil, "s"); n != 5 {
		t.Fatalf("switch on harus jalan, dapat %d", n)
	}
}

// TestGraphProjectionFailsOpen — proyeksi error di-skip, gak ganggu yg sehat.
func TestGraphProjectionFailsOpen(t *testing.T) {
	saved := extraGraphProjections
	extraGraphProjections = nil
	defer func() { extraGraphProjections = saved }()

	RegisterGraphProjection(GraphProjection{
		Name: "boom",
		Run:  func(ctx context.Context, store *agentdb.Store, scope string) (int, error) { return 0, context.Canceled },
	})
	RegisterGraphProjection(GraphProjection{
		Name: "ok",
		Run:  func(ctx context.Context, store *agentdb.Store, scope string) (int, error) { return 4, nil },
	})
	if n := runExtraGraphProjections(context.Background(), nil, "s"); n != 4 {
		t.Fatalf("fails-open: error di-skip, sehat tetap jalan; mau 4, dapat %d", n)
	}
}
