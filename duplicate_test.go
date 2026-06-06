package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowork-gui/internal/agentdb"
)

// TestAgentDuplicate is hermetic: it builds a fake source agent in a temp
// AgentsDir, duplicates it, and checks the copy got the wasm + a rewritten
// manifest + the config persona, but NOT any secrets.
func TestAgentDuplicate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", tmp)

	src := filepath.Join(tmp, "src-agent.fwagent")
	if err := os.MkdirAll(filepath.Join(src, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(src, "agent.wasm"), []byte("\x00asm-dummy"), 0o644)
	_ = os.WriteFile(filepath.Join(src, "manifest.json"),
		[]byte(`{"id":"src-agent","display_name":"Src","kind":"agent","entry":"agent.wasm","capabilities_required":["state:write"]}`), 0o644)
	// seed config: a persona + a secret that must NOT be copied.
	if st, err := agentdb.Open(filepath.Join(src, "workspace", "state.db")); err == nil {
		_ = st.Save(map[string]any{
			"prompt":  "you are the source persona",
			"secrets": map[string]any{"API_KEY": "should-not-copy"},
		})
		st.Close()
	} else {
		t.Fatalf("seed src db: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/agents/duplicate?id=src-agent&new_id=dst-agent", nil)
	rec := httptest.NewRecorder()
	agentDuplicateHandler(rec, req)

	dst := filepath.Join(tmp, "dst-agent.fwagent")
	if _, err := os.Stat(filepath.Join(dst, "agent.wasm")); err != nil {
		t.Fatalf("dst wasm missing — resp=%s", rec.Body.String())
	}
	man, _ := os.ReadFile(filepath.Join(dst, "manifest.json"))
	if !strings.Contains(string(man), `"id": "dst-agent"`) {
		t.Errorf("manifest id not rewritten: %s", man)
	}
	if !strings.Contains(string(man), "(copy)") {
		t.Errorf("display_name not marked copy: %s", man)
	}

	st, err := agentdb.Open(filepath.Join(dst, "workspace", "state.db"))
	if err != nil {
		t.Fatalf("open dst db: %v", err)
	}
	cfg, _ := st.Load()
	st.Close()
	if cfg["prompt"] != "you are the source persona" {
		t.Errorf("persona not copied: %v", cfg["prompt"])
	}
	if _, leaked := cfg["secrets"]; leaked {
		t.Error("SECURITY: secrets were copied into the duplicate")
	}
}

func TestAgentDuplicateRejects(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", tmp)
	bad := func(q string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/agents/duplicate?"+q, nil)
		rec := httptest.NewRecorder()
		agentDuplicateHandler(rec, req)
		return rec.Code
	}
	// invalid new_id, missing source, same id → all non-2xx OR error body.
	if c := bad("id=mr-flow&new_id=Bad_ID"); c == http.StatusOK {
		// tfWriteJSON uses explicit status; bad id should be 400.
		t.Errorf("invalid new_id accepted (code=%d)", c)
	}
}
