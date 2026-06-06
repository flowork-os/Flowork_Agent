package mcphub

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowork-gui/internal/tools"
)

// Dogfood Phase 2: install an MCP connector pointing at our own flowork-mcp server,
// enable it, and confirm its tools land in the engine registry (agent-reachable) and
// disappear on disable. If the live flowork-gui is up, also run a bridged tool.
func TestMCPHub_EnableRegistersTools(t *testing.T) {
	bin, _ := filepath.Abs("../../bin/flowork-mcp")
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("flowork-mcp not built: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	const id = "dogfood"

	if err := Install(id, SavedConfig{Command: bin}); err != nil {
		t.Fatal(err)
	}
	if l := Default.List(); len(l) != 1 || l[0].ID != id {
		t.Fatalf("list wrong: %+v", l)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Default.Enable(ctx, id); err != nil {
		t.Fatalf("enable: %v", err)
	}
	defer Default.Disable(id)

	// the MCP server's tools must now be in the engine registry as mcp_<id>_<tool>
	want := "mcp_dogfood_chat"
	if _, ok := tools.Lookup(want); !ok {
		t.Fatalf("bridged tool %q not in registry; names=%v", want, Default.ToolsFor(id))
	}

	// run it through the registry (needs the live server for a real reply)
	cctx, ccancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer ccancel()
	if tl, ok := tools.Lookup(want); ok {
		res, err := tl.Run(cctx, map[string]any{"message": "halo, 1 kata"})
		if err != nil {
			t.Logf("run (server may be down): %v", err)
		} else {
			t.Logf("bridged run output: %v", res.Output)
		}
	}

	// disable → tool unregistered
	if err := Default.Disable(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := tools.Lookup(want); ok {
		t.Errorf("tool %q still registered after disable", want)
	}
}
