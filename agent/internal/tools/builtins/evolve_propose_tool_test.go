package builtins

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

// TestEvolveProposeForgiving — LLM (mr-flow) sering manggil pakai title/description, bukan
// target_file/rationale. Tool WAJIB nerima itu + auto-turunin NEW:<slug> buat behavior.
func TestEvolveProposeForgiving(t *testing.T) {
	store, err := agentdb.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	ctx := tools.WithStore(context.Background(), store)

	// (1) panggilan NATURAL: cuma title + description (persis cara mr-flow manggil) → HARUS lolos.
	res, err := evolveProposeTool{}.Run(ctx, map[string]any{
		"title":       "Cek koneksi router sebelum LLM",
		"description": "biar agent ga gagal tengah jalan pas router lagi ngadat",
	})
	if err != nil {
		t.Fatalf("panggilan natural (title+description) DITOLAK, harusnya lolos: %v", err)
	}
	out, _ := res.Output.(map[string]any)
	if out["ok"] != true {
		t.Fatalf("ok != true: %v", out)
	}
	tgt, _ := out["target_file"].(string)
	if !strings.HasPrefix(tgt, "NEW:") {
		t.Fatalf("target_file behavior harusnya auto 'NEW:<slug>', dapet %q", tgt)
	}
	if k, _ := out["kind"].(string); k != "add-skill" {
		t.Fatalf("kind default harusnya add-skill, dapet %q", k)
	}

	// proposal beneran tersimpan (status proposed, rationale keisi dari description).
	props, err := store.ListEvolveProposals(10)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(props) != 1 {
		t.Fatalf("harusnya 1 proposal tersimpan, dapet %d", len(props))
	}
	if r, _ := props[0]["rationale"].(string); strings.TrimSpace(r) == "" {
		t.Fatalf("rationale kosong (harusnya keisi dari description)")
	}

	// (2) core (kind=fix) TANPA target_file → HARUS ditolak (core wajib path repo asli).
	if _, err := (evolveProposeTool{}).Run(ctx, map[string]any{
		"kind":      "fix",
		"rationale": "benerin sesuatu",
	}); err == nil {
		t.Fatalf("core (fix) tanpa target_file harusnya DITOLAK, tapi lolos")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Cek Koneksi Router": "cek-koneksi-router",
		"  Foo/Bar_baz  ":    "foo-bar-baz",
		"!!!":                "ide-evolusi",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q)=%q, mau %q", in, got, want)
		}
	}
}
