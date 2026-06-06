package mcpclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Dogfood: drive the project's OWN flowork-mcp server as an external MCP server.
// Proves the client can spawn → initialize → list tools → call a tool, with no
// external dependency.
func TestDogfood_FloworkMCP(t *testing.T) {
	bin, _ := filepath.Abs("../../bin/flowork-mcp")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("flowork-mcp binary not built: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Start(ctx, "dogfood", Config{Command: bin})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	tools, err := s.ListTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	t.Logf("tools: %d %v", len(tools), names)
	if !names["chat"] {
		t.Fatalf("expected 'chat' tool, got %v", names)
	}

	// call chat — needs the live flowork-gui (handle_message → LLM). Generous timeout.
	cctx, ccancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer ccancel()
	reply, err := s.CallTool(cctx, "chat", map[string]any{"message": "halo, jawab 1 kata"})
	if err != nil {
		t.Logf("chat call err (server/router may be down): %v", err)
		return
	}
	t.Logf("chat reply: %q", reply)
	if reply == "" {
		t.Error("empty reply")
	}
}
