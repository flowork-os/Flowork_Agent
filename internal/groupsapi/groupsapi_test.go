package groupsapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowork-gui/internal/loket"
)

// seed writes group config into a module's loket store at <dir>/<id>.db.
func seed(t *testing.T, dir, id string, kv map[string]string) {
	t.Helper()
	st, err := loket.OpenStore(filepath.Join(dir, id+".db"))
	if err != nil {
		t.Fatalf("open store %s: %v", id, err)
	}
	defer st.Close()
	for k, v := range kv {
		if err := st.KVSet(k, v); err != nil {
			t.Fatalf("kvset %s.%s: %v", id, k, err)
		}
	}
}

func newHandler(dir string, ids []string) *Handler {
	return New(Deps{
		AgentIDs:       func() []string { return ids },
		LoketStorePath: func(m string) (string, error) { return filepath.Join(dir, m+".db"), nil },
		AgentsDir:      dir, // no manifests → displayName falls back to id (fine for test)
		GroupWasmPath:  filepath.Join(dir, "tpl.wasm"),
	})
}

func TestListGroupsAndAvailable(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, "trading-group", map[string]string{
		"group": "1", "members": "analis-plus, analis-minus", "synthesizer": "analis-sinteser", "task": "analisa",
	})
	seed(t, dir, "analis-plus", map[string]string{"prompt": "x"}) // not a group
	h := newHandler(dir, []string{"trading-group", "analis-plus", "analis-minus", "analis-sinteser"})

	rec := httptest.NewRecorder()
	h.ListHandler(rec, httptest.NewRequest(http.MethodGet, "/api/groups", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var out struct {
		Groups []struct {
			ID          string   `json:"id"`
			Members     []string `json:"members"`
			Synthesizer string   `json:"synthesizer"`
		} `json:"groups"`
		Available []struct {
			ID string `json:"id"`
		} `json:"available_agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Groups) != 1 || out.Groups[0].ID != "trading-group" {
		t.Fatalf("groups = %+v", out.Groups)
	}
	if len(out.Groups[0].Members) != 2 || out.Groups[0].Synthesizer != "analis-sinteser" {
		t.Fatalf("roster = %+v", out.Groups[0])
	}
	// The three non-group modules must appear as available members.
	if len(out.Available) != 3 {
		t.Fatalf("available = %+v", out.Available)
	}
}

func TestConfigWritesRosterLive(t *testing.T) {
	dir := t.TempDir()
	h := newHandler(dir, []string{"g1"})

	body := `{"members":["a","b","g1"," "],"synthesizer":"s","task":"do it"}` // g1 (self) + blank must be dropped
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/groups/config?id=g1", strings.NewReader(body))
	h.ConfigHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	// Read back via a fresh store: members must be "a,b" (self + blank filtered).
	st, _ := loket.OpenStore(filepath.Join(dir, "g1.db"))
	defer st.Close()
	if v, _, _ := st.KVGet("group"); v != "1" {
		t.Fatalf("group marker = %q", v)
	}
	if v, _, _ := st.KVGet("members"); v != "a,b" {
		t.Fatalf("members = %q (self/blank not filtered)", v)
	}
	if v, _, _ := st.KVGet("synthesizer"); v != "s" {
		t.Fatalf("synthesizer = %q", v)
	}
}

func TestCreateGroupDeploysFolderAndMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tpl.wasm"), []byte("\x00asm-stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHandler(dir, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/groups/create", strings.NewReader(`{"id":"group-trading","display_name":"Group Trading"}`))
	h.CreateHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	fw := filepath.Join(dir, "group-trading.fwagent")
	if _, err := os.Stat(filepath.Join(fw, "manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fw, "agent.wasm")); err != nil {
		t.Fatalf("wasm missing: %v", err)
	}
	st, _ := loket.OpenStore(filepath.Join(dir, "group-trading.db"))
	defer st.Close()
	if v, _, _ := st.KVGet("group"); v != "1" {
		t.Fatalf("group marker = %q", v)
	}

	// Bad id → rejected; duplicate → 409.
	rec2 := httptest.NewRecorder()
	h.CreateHandler(rec2, httptest.NewRequest(http.MethodPost, "/api/groups/create", strings.NewReader(`{"id":"../escape"}`)))
	if rec2.Code != 400 {
		t.Fatalf("bad id status = %d", rec2.Code)
	}
	rec3 := httptest.NewRecorder()
	h.CreateHandler(rec3, httptest.NewRequest(http.MethodPost, "/api/groups/create", strings.NewReader(`{"id":"group-trading"}`)))
	if rec3.Code != 409 {
		t.Fatalf("dup status = %d", rec3.Code)
	}
}

func TestDeleteOnlyGroups(t *testing.T) {
	dir := t.TempDir()
	// A real GROUP folder + marker.
	if err := os.MkdirAll(filepath.Join(dir, "grp.fwagent"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed(t, dir, "grp", map[string]string{"group": "1"})
	// A plain agent (NOT a group) — must be refused.
	if err := os.MkdirAll(filepath.Join(dir, "mr-flow.fwagent"), 0o755); err != nil {
		t.Fatal(err)
	}
	seed(t, dir, "mr-flow", map[string]string{"prompt": "x"})
	h := newHandler(dir, []string{"grp", "mr-flow"})

	// Refuse deleting a non-group (protects real agents).
	rec := httptest.NewRecorder()
	h.DeleteHandler(rec, httptest.NewRequest(http.MethodPost, "/api/groups/delete?id=mr-flow", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-group delete status = %d (want 403)", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "mr-flow.fwagent")); err != nil {
		t.Fatalf("mr-flow folder wrongly removed: %v", err)
	}
	// Delete the real group.
	rec2 := httptest.NewRecorder()
	h.DeleteHandler(rec2, httptest.NewRequest(http.MethodPost, "/api/groups/delete?id=grp", nil))
	if rec2.Code != 200 {
		t.Fatalf("group delete status = %d", rec2.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "grp.fwagent")); !os.IsNotExist(err) {
		t.Fatalf("group folder not removed")
	}
}
