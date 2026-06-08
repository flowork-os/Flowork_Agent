package builtins

import (
	"context"
	"path/filepath"
	"testing"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

// TestAgentRunLifecycle walks the full stop+resume lifecycle deterministically:
// create → start → checkpoint → pause → resume(returns checkpoint) → stop.
func TestAgentRunLifecycle(t *testing.T) {
	store, err := agentdb.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := tools.WithStore(context.Background(), store)
	tool := agentRunTool{}
	run := func(args map[string]any) map[string]any {
		res, err := tool.Run(ctx, args)
		if err != nil {
			t.Fatalf("action %v: %v", args["action"], err)
		}
		return res.Output.(map[string]any)
	}

	run(map[string]any{"action": "create", "id": "job1", "label": "demo"})
	if s := run(map[string]any{"action": "start", "id": "job1"})["state"]; s != "running" {
		t.Fatalf("start state = %v", s)
	}

	// checkpoint partial progress
	run(map[string]any{"action": "checkpoint", "id": "job1", "data": map[string]any{"step": 3, "note": "halfway"}})
	if s := run(map[string]any{"action": "pause", "id": "job1"})["state"]; s != "paused" {
		t.Fatalf("pause state = %v", s)
	}

	// resume must come back running AND hand back the saved checkpoint
	resumed := run(map[string]any{"action": "resume", "id": "job1"})
	if resumed["state"] != "running" {
		t.Fatalf("resume state = %v", resumed["state"])
	}
	cp, ok := resumed["checkpoint"].(map[string]any)
	if !ok || cp["note"] != "halfway" {
		t.Fatalf("resume should return saved checkpoint, got %v", resumed["checkpoint"])
	}

	// stop is terminal + enforceable: status reflects it, and resume won't revive it
	if s := run(map[string]any{"action": "stop", "id": "job1"})["state"]; s != "stopped" {
		t.Fatalf("stop state = %v", s)
	}
	if s := run(map[string]any{"action": "status", "id": "job1"})["state"]; s != "stopped" {
		t.Fatalf("status after stop = %v", s)
	}
	if s := run(map[string]any{"action": "resume", "id": "job1"})["state"]; s != "stopped" {
		t.Fatalf("resume must not revive a stopped run, got %v", s)
	}

	// unknown id errors; list returns the run
	if _, err := tool.Run(ctx, map[string]any{"action": "status", "id": "ghost"}); err == nil {
		t.Fatal("status on unknown id should error")
	}
	if runs := run(map[string]any{"action": "list"})["runs"].([]map[string]any); len(runs) != 1 {
		t.Fatalf("list should have 1 run, got %d", len(runs))
	}
}
