package settingsapi

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"flowork-gui/internal/floworkdb"
)

// TestKeysPostRejectsEmptyValue proves the data-integrity guard: the GUI clears the
// value field on "Edit", so a POST with an empty value must NOT overwrite (wipe) the
// stored secret — it must be rejected, and the real value left intact.
func TestKeysPostRejectsEmptyValue(t *testing.T) {
	store, err := floworkdb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if err := store.SetSecret("MY_TOKEN", "real-secret-123"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a := New(store)

	req := httptest.NewRequest("POST", "/api/settings/keys", strings.NewReader(`{"key":"MY_TOKEN","value":""}`))
	w := httptest.NewRecorder()
	a.KeysHandler(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == nil {
		t.Fatalf("empty value must be rejected, got: %v", resp)
	}
	if v, _ := store.GetSecret("MY_TOKEN"); v != "real-secret-123" {
		t.Fatalf("stored secret was clobbered by an empty POST: got %q", v)
	}

	// sanity: a real value still saves
	req2 := httptest.NewRequest("POST", "/api/settings/keys", strings.NewReader(`{"key":"MY_TOKEN","value":"new-value-456"}`))
	w2 := httptest.NewRecorder()
	a.KeysHandler(w2, req2)
	if v, _ := store.GetSecret("MY_TOKEN"); v != "new-value-456" {
		t.Fatalf("a real value must save: got %q", v)
	}
}
