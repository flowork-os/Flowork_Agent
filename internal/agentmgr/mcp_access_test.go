package agentmgr

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPExcluded_Roundtrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", root)
	const agent = "test-agent"
	if err := os.MkdirAll(filepath.Join(root, agent+".fwagent"), 0o755); err != nil {
		t.Fatal(err)
	}
	// storage roundtrip
	if err := setMCPExcluded(agent, []string{"github", "filesystem"}); err != nil {
		t.Fatal(err)
	}
	got := mcpExcludedSet(agent)
	if !got["github"] || !got["filesystem"] {
		t.Fatalf("excluded set wrong: %v", got)
	}
	// handler POST clears
	r := httptest.NewRequest("POST", "/api/agents/mcp?id="+agent, strings.NewReader(`{"excluded":[]}`))
	w := httptest.NewRecorder()
	AgentMCPHandler(w, r)
	if rc := w.Code; rc >= 400 {
		t.Fatalf("POST failed: %d %s", rc, w.Body.String())
	}
	if len(mcpExcludedSet(agent)) != 0 {
		t.Error("exclusion not cleared after POST []")
	}
	// invalid id rejected
	rr := httptest.NewRequest("GET", "/api/agents/mcp?id=../x", nil)
	ww := httptest.NewRecorder()
	AgentMCPHandler(ww, rr)
	if !strings.Contains(ww.Body.String(), "invalid") {
		t.Error("traversal id not rejected")
	}
}
