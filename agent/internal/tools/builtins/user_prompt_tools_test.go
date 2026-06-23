package builtins

import (
	"context"
	"path/filepath"
	"testing"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

func TestUserPromptToolsRegistered(t *testing.T) {
	// Call Init() to register all builtin tools statically defined in builtins.go
	// (Normally called once during main.go startup)
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Init() already called elsewhere: %v", r)
		}
	}()
	Init()

	requiredTools := []string{
		"instinct_recall",
		"mistake_recall",
		"interaction_recall",
		"memory_get",
		"skill_search",
		"graph_recall",
		"brain_search",
		"cognitive_tensions",
		"cognitive_resolve",
		"web_search",
		"webfetch",
		"task_list",
		"task_run",
		"tool_search",
	}

	for _, name := range requiredTools {
		tool, ok := tools.Lookup(name)
		if !ok {
			t.Errorf("Tool %q NOT registered in the agent registry!", name)
		} else {
			t.Logf("Tool %q is registered. Schema Description: %q", name, tool.Schema().Description)
		}
	}
}

func TestUserPromptToolsExecution(t *testing.T) {
	s, err := agentdb.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := tools.WithStore(context.Background(), s)

	// Call Init() to register all builtin tools statically defined in builtins.go
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Init() already called elsewhere: %v", r)
		}
	}()
	Init()

	testCases := []struct {
		name string
		args map[string]any
	}{
		{"instinct_recall", map[string]any{"query": "coding task"}},
		{"mistake_recall", map[string]any{"context": "some context"}},
		{"interaction_recall", map[string]any{}},
		{"memory_get", map[string]any{"key": "test_key"}},
		{"skill_search", map[string]any{"query": ""}},
		{"graph_recall", map[string]any{"query": "grounding text"}},
		{"brain_search", map[string]any{"query": "fact search"}},
		{"cognitive_tensions", map[string]any{}},
		{"cognitive_resolve", map[string]any{"tension_id": float64(1), "keep": "new"}},
		{"web_search", map[string]any{"query": "test query"}},
		{"webfetch", map[string]any{"url": "https://google.com"}},
		{"task_list", map[string]any{}},
		{"task_run", map[string]any{"category": "test", "subject": "test"}},
		{"tool_search", map[string]any{"query": "brain"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tool, ok := tools.Lookup(tc.name)
			if !ok {
				t.Fatalf("Tool %q not found in registry", tc.name)
			}

			// We wrap in a recover block to catch any unexpected panics
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Tool %q PANICKED: %v", tc.name, r)
				}
			}()

			_, err := tool.Run(ctx, tc.args)
			// We don't check for err != nil since many tools will correctly return errors
			// in the test environment (e.g. missing HTTP servers or DB entries),
			// but we want to make sure they do not panic and they handle execution gracefully.
			t.Logf("Tool %q ran. Err: %v", tc.name, err)
		})
	}
}
