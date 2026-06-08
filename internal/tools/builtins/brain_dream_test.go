package builtins

import (
	"context"
	"path/filepath"
	"testing"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

func TestBrainDream(t *testing.T) {
	store, err := agentdb.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Seed two never-recalled memories (amplitude 0, default importance).
	if _, _, err := store.AddBrainDrawer("memory one", "experience", "", "experience", "agent"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, _, err := store.AddBrainDrawer("memory two", "experience", "", "experience", "agent"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var before float64
	_ = store.DB().QueryRow("SELECT MIN(importance) FROM brain_drawers").Scan(&before)

	ctx := tools.WithStore(context.Background(), store)
	res, err := (brainDreamTool{}).Run(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("dream: %v", err)
	}
	out := res.Output.(map[string]any)
	if out["decayed"].(int64) != 2 {
		t.Fatalf("expected 2 decayed, got %v", out["decayed"])
	}
	var after float64
	_ = store.DB().QueryRow("SELECT MIN(importance) FROM brain_drawers").Scan(&after)
	if !(after < before) {
		t.Fatalf("importance should decay: before=%v after=%v", before, after)
	}

	// Empty/fresh store → no error, 0 decayed (graceful when there's no brain yet).
	store2, _ := agentdb.Open(filepath.Join(t.TempDir(), "s2.db"))
	defer store2.Close()
	ctx2 := tools.WithStore(context.Background(), store2)
	if r2, err := (brainDreamTool{}).Run(ctx2, nil); err != nil {
		t.Fatalf("fresh store should not error: %v", err)
	} else if r2.Output.(map[string]any)["decayed"].(int64) != 0 {
		t.Fatalf("fresh store should decay 0")
	}
}
