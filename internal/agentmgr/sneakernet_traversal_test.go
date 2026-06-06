package agentmgr

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSneakernet_RejectTraversal(t *testing.T) {
	// export
	r := httptest.NewRequest("POST", "/api/agents/sneakernet/export?id=../../etc", nil)
	w := httptest.NewRecorder()
	SneakernetExportHandler(w, r)
	if !strings.Contains(w.Body.String(), "invalid") {
		t.Errorf("export traversal not rejected: %s", w.Body.String())
	}
	// import
	r2 := httptest.NewRequest("POST", "/api/agents/sneakernet/import?target_id=../../tmp/x", strings.NewReader("{}"))
	w2 := httptest.NewRecorder()
	SneakernetImportHandler(w2, r2)
	if !strings.Contains(w2.Body.String(), "invalid") {
		t.Errorf("import traversal not rejected: %s", w2.Body.String())
	}
}
