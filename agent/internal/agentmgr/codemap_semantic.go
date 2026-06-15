// codemap_semantic.go — R6 SELF-MAP SEMANTIK (handler + enrich engine, plug-in).
// Owner-approved 2026-06-15 (FASE 2 autonomi). Di ATAS self-map deterministik
// (codemap_files + WalkRepo), tambah lapisan MAKNA: tiap file node → analisa LLM kecil
// (prinsip semut: 1 file = 1 prompt fokus, ramah LLM lokal) → simpan summary/domain/role.
// Gak sentuh legacy_compat.go / codemap.go (LOCKED). LLM di-INJECT dari main (decoupling).

package agentmgr

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

// SemanticSummarizer — di-inject dari main (pakai routerChat). Analisa 1 file → makna.
// model="" → biar implementasi resolve default (Settings/env). usedModel = provenance.
type SemanticSummarizer func(ctx context.Context, path, content, model string) (summary, domain, role, usedModel string, err error)

// CodemapEnrichHandler — POST /api/codemap/enrich?limit=&force=&model=
// Lapisan MAKNA self-map: ambil file node deterministik → enrich LLM → simpan semantic.
// INCREMENTAL: skip file yang udah ke-enrich (kecuali force=1) → batch demi batch aman
// (panggil berkali2 sampe kelar; default limit 20/batch). Owner-gated (di-wrap auth main).
func CodemapEnrichHandler(summarize SemanticSummarizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
			return
		}
		if summarize == nil {
			httpx.WriteJSON(w, map[string]any{"error": "summarizer not wired"})
			return
		}
		limit := parseLimitOr(r.URL.Query().Get("limit"), 20)
		force := r.URL.Query().Get("force") == "1"
		model := strings.TrimSpace(r.URL.Query().Get("model"))

		store, err := openAgentStore(defaultAgentID)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		defer store.Close()
		files, err := store.ListCodemapFiles()
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		if len(files) == 0 {
			httpx.WriteJSON(w, map[string]any{"error": "self-map kosong — jalanin /api/codemap/reindex dulu"})
			return
		}
		done := map[string]bool{}
		if !force {
			done, _ = store.CodemapSemanticPaths()
		}
		root := codemapRoot()
		ctx, cancel := context.WithTimeout(r.Context(), 290*time.Second)
		defer cancel()

		enriched, skipped, failed := 0, 0, 0
		for _, f := range files {
			if enriched >= limit {
				break
			}
			path, _ := f["path"].(string)
			if path == "" {
				continue
			}
			if !force && done[path] {
				skipped++
				continue
			}
			// baca file (anti-traversal + cap ukuran biar prompt kecil = ramah LLM lokal).
			clean := filepath.Clean(filepath.Join(root, path))
			if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
				failed++
				continue
			}
			data, rerr := os.ReadFile(clean)
			if rerr != nil {
				failed++
				continue
			}
			const maxBytes = 24 * 1024
			if len(data) > maxBytes {
				data = data[:maxBytes]
			}
			sum, dom, role, used, serr := summarize(ctx, path, string(data), model)
			if serr != nil || strings.TrimSpace(sum) == "" {
				failed++
				if ctx.Err() != nil {
					break // timeout total → stop batch, hasil parsial tetap kesimpan
				}
				continue
			}
			if uerr := store.UpsertCodemapSemantic(agentdb.CodemapSemantic{
				Path: path, Summary: sum, Domain: dom, Role: role, Model: used,
			}); uerr != nil {
				failed++
				continue
			}
			enriched++
		}
		// hitung sisa yang belum ke-enrich (buat owner tahu progres).
		paths, _ := store.CodemapSemanticPaths()
		remaining := 0
		for _, f := range files {
			if p, _ := f["path"].(string); p != "" && !paths[p] {
				remaining++
			}
		}
		httpx.WriteJSON(w, map[string]any{
			"ok": true, "enriched": enriched, "skipped": skipped, "failed": failed,
			"total_files": len(files), "remaining": remaining,
		})
	}
}

// CodemapSemanticHandler — GET /api/codemap/semantic → lapisan makna (GUI viz + R7 konsumsi).
func CodemapSemanticHandler(w http.ResponseWriter, r *http.Request) {
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListCodemapSemantic()
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}
