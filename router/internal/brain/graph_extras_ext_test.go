package brain

import (
	"context"
	"database/sql"
	"testing"
)

// TestRegisterGraphProjectionRouter — seam ke-trigger + nil-Run aman + total ke-akumulasi.
func TestRegisterGraphProjectionRouter(t *testing.T) {
	saved := extraGraphProjections
	extraGraphProjections = nil
	defer func() { extraGraphProjections = saved }()

	called := false
	RegisterGraphProjection(GraphProjection{
		Name: "test",
		Run:  func(ctx context.Context, tx *sql.Tx) (int, error) { called = true; return 2, nil },
	})
	RegisterGraphProjection(GraphProjection{Name: "nilrun"}) // diabaikan

	n := runExtraGraphProjectionsTx(context.Background(), nil)
	if !called {
		t.Fatal("proyeksi terdaftar tidak dipanggil")
	}
	if n != 2 {
		t.Fatalf("total: mau 2, dapat %d", n)
	}
}

// TestRouterProjectionSwitch — switch OFF skip, ON jalan.
func TestRouterProjectionSwitch(t *testing.T) {
	saved := extraGraphProjections
	extraGraphProjections = nil
	defer func() { extraGraphProjections = saved }()

	RegisterGraphProjection(GraphProjection{
		Name:   "g",
		Switch: "FLOWORK_TEST_DG_X",
		Run:    func(ctx context.Context, tx *sql.Tx) (int, error) { return 7, nil },
	})
	t.Setenv("FLOWORK_TEST_DG_X", "off")
	if n := runExtraGraphProjectionsTx(context.Background(), nil); n != 0 {
		t.Fatalf("off harus skip, dapat %d", n)
	}
	t.Setenv("FLOWORK_TEST_DG_X", "on")
	if n := runExtraGraphProjectionsTx(context.Background(), nil); n != 7 {
		t.Fatalf("on harus jalan, dapat %d", n)
	}
}
