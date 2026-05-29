// codemap_reindex.go — Reindex, Status, Review, GitHook handlers.
package guiapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/codeindex"
)

// CodemapReindexHandler POST /api/codemap/reindex
// Trigger full re-index, incremental (?mode=incremental), atau partial (?file=...).
func CodemapReindexHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}

		ix, err := getOrCreateIndexer(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		file := r.URL.Query().Get("file")
		if file != "" {
			if err := ix.IndexFile(filepath.Join(ws, filepath.FromSlash(file))); err != nil {
				safeError(w, http.StatusInternalServerError, "internal error", err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "file": file})
			return
		}

		// BUG-013 fix: atomic TOCTOU guard — hold codemapMu from check to goroutine spawn.
		codemapMu.Lock()
		if ix.IsRunning() || indexerStarting {
			codemapMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "already running"})
			return
		}
		indexerStarting = true
		codemapMu.Unlock()

		mode := r.URL.Query().Get("mode")
		isIncremental := mode == "incremental"

		go func() {
			codemapMu.Lock()
			lastReindex = time.Now()
			indexerStarting = false
			codemapMu.Unlock()

			var stats *codeindex.IndexStats
			var indexErr error
			if isIncremental {
				stats, indexErr = ix.IncrementalUpdate()
			} else {
				stats, indexErr = ix.IndexAll()
			}
			codemapMu.Lock()
			if indexErr == nil {
				lastStats = stats
			}
			codemapMu.Unlock()
		}()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"mode":    mode,
			"message": "reindex started",
		})
	}
}

// CodemapStatusHandler GET /api/codemap/status
func CodemapStatusHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ix, err := getOrCreateIndexer(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		codemapMu.Lock()
		st := lastStats
		lr := lastReindex
		codemapMu.Unlock()

		resp := map[string]any{
			"running": ix.IsRunning(),
		}
		if !lr.IsZero() {
			resp["last_reindex"] = lr.Format(time.RFC3339)
		}
		if st != nil {
			resp["last_stats"] = map[string]any{
				"files_indexed": st.FilesIndexed,
				"files_skipped": st.FilesSkipped,
				"edges_created": st.EdgesCreated,
				"errors":        st.Errors,
				"duration_ms":   st.Duration.Milliseconds(),
				"incremental":   st.Incremental,
			}
		}

		brainDB, _ := db.Shared(ws)
		if brainDB != nil {
			var nodeCount, edgeCount int
			brainDB.QueryRow(`SELECT COUNT(*) FROM codemap_nodes`).Scan(&nodeCount)
			brainDB.QueryRow(`SELECT COUNT(*) FROM codemap_edges`).Scan(&edgeCount)
			resp["node_count"] = nodeCount
			resp["edge_count"] = edgeCount
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// CodemapReviewHandler POST /api/codemap/review
// CRG-inspired: generate minimal review context for changed files.
func CodemapReviewHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		brainDB, err := db.Shared(ws)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		var changedFiles []string

		if r.URL.Query().Get("git") == "1" {
			files, err := codeindex.DetectGitChangedFiles(ws)
			if err != nil {
				safeError(w, http.StatusInternalServerError, "git diff failed", err)
				return
			}
			for _, f := range files {
				rel, _ := filepath.Rel(ws, f)
				changedFiles = append(changedFiles, filepath.ToSlash(rel))
			}
		} else {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			var body struct {
				Files []string `json:"files"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			changedFiles = body.Files
		}

		if len(changedFiles) == 0 {
			http.Error(w, "no changed files specified", 400)
			return
		}

		ctx, err := codeindex.BuildReviewContext(brainDB, changedFiles, 3)
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ctx)
	}
}

// CodemapGitHookHandler POST /api/codemap/githook
// Install atau uninstall git post-commit hook.
func CodemapGitHookHandler(ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		action := r.URL.Query().Get("action")
		var err error
		switch action {
		case "uninstall":
			err = codeindex.UninstallGitHook(ws)
		default:
			err = codeindex.InstallGitHook(ws)
		}
		if err != nil {
			safeError(w, http.StatusInternalServerError, "internal error", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "action": action})
	}
}
