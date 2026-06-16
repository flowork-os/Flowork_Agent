package brain

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

// TestSemanticLive — integrasi ARSITEK BARU: butuh index (FLOWORK_BRAIN_VINDEX) + brain db
// (FLOW_ROUTER_BRAIN_DB) + Ollama bge-m3. Skip kalau gak ada. Buktiin SemanticRetrieve balik
// memori by-MAKNA (vector murni).
func TestSemanticLive(t *testing.T) {
	idxP := os.Getenv("FLOWORK_BRAIN_VINDEX")
	dbP := os.Getenv("FLOW_ROUTER_BRAIN_DB")
	if idxP == "" || dbP == "" {
		t.Skip("set FLOWORK_BRAIN_VINDEX + FLOW_ROUTER_BRAIN_DB buat integrasi")
	}
	db, err := sql.Open("sqlite", "file:"+dbP+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if !VectorReady() {
		t.Fatalf("index gak ke-load dari %s", idxP)
	}
	// vectorRetrieve langsung (vector murni) — buktiin bukan FTS fallback.
	vec := vectorRetrieve(context.Background(), db, "honeypot rug pull scam token", 5, 100)
	t.Logf("vector murni balik %d snippet (by-makna):", len(vec))
	for i, s := range vec {
		t.Logf("  %d. [%.4f] (%s) %s", i+1, s.Score, s.Wing, s.Content)
	}
	if len(vec) == 0 {
		t.Error("vector kosong — index ada tapi gak balik hasil")
	}
	// SemanticRetrieve full (harus = vector kalau index siap).
	snips, err := SemanticRetrieve(context.Background(), db, "deteksi token penipuan crypto", RetrieveOpts{Limit: 3, MaxContentLen: 80})
	if err != nil {
		t.Fatal(err)
	}
	if len(snips) == 0 {
		t.Error("SemanticRetrieve kosong")
	}
}
