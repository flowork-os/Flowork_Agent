package connections

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise the HTTP layer (the surface the GUI calls) end-to-end against a real
// installed connector, so the wiring — not just the core funcs — is covered.
func TestHandlers_RoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FLOWORK_AGENTS_DIR", root)
	const id = "http-conn"
	if _, st := InstallChannelPack(buildChannelPack(t, id, nil)); st != 0 {
		t.Fatal("install failed")
	}

	// GET /api/connections → list contains the connector
	rec := httptest.NewRecorder()
	ListHandler(rec, httptest.NewRequest(http.MethodGet, "/api/connections", nil))
	if !strings.Contains(rec.Body.String(), id) {
		t.Fatalf("list missing connector: %s", rec.Body.String())
	}

	// POST /api/connections/config → set a token, GET masks it
	post(t, ConfigHandler, http.MethodPost, "/api/connections/config",
		`{"id":"`+id+`","config":{"BOT_TOKEN":"abcd1234secret","TARGET_AGENT":"mr-flow-next"}}`)
	rec = httptest.NewRecorder()
	ConfigHandler(rec, httptest.NewRequest(http.MethodGet, "/api/connections/config?id="+id, nil))
	var cfgResp struct {
		Config map[string]string `json:"config"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &cfgResp)
	if cfgResp.Config["BOT_TOKEN"] == "abcd1234secret" {
		t.Error("token not masked over HTTP")
	}

	// POST /api/connections/toggle → disable, reflected in IsEnabled
	post(t, ToggleHandler, http.MethodPost, "/api/connections/toggle", `{"id":"`+id+`","enabled":false}`)
	if IsEnabled(id) {
		t.Error("connector still enabled after toggle off")
	}

	// POST /api/connections/uninstall → folder gone
	post(t, UninstallHandler, http.MethodPost, "/api/connections/uninstall", `{"id":"`+id+`"}`)
	if findConn(id) != nil {
		t.Error("connector still listed after uninstall")
	}
}

func post(t *testing.T, h http.HandlerFunc, method, path, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if rec.Code >= 400 {
		t.Fatalf("%s %s -> %d: %s", method, path, rec.Code, rec.Body.String())
	}
}
