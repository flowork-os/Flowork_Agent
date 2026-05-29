// codemap_indexer.go — Singleton indexer state + shared helpers.
package guiapi

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/codeindex"
)

var (
	codemapMu      sync.Mutex
	codemapIndexer *codeindex.Indexer
	lastReindex    time.Time
	lastStats      *codeindex.IndexStats

	// indexerStarting — BUG-013 fix: gap-bridging guard antara HTTP handler
	// return + IndexAll().mu.Lock acquire di goroutine. Protected by codemapMu.
	indexerStarting bool
)

func getOrCreateIndexer(ws string) (*codeindex.Indexer, error) {
	codemapMu.Lock()
	defer codemapMu.Unlock()
	if codemapIndexer != nil {
		return codemapIndexer, nil
	}
	brainDB, err := db.Shared(ws)
	if err != nil {
		return nil, err
	}
	codemapIndexer = codeindex.NewIndexer(brainDB, ws)
	// 2026-05-06 (Ayah audit "map untuk semua file di dalam project"):
	// auto-add sibling repos di project root supaya Code Map cover SELURUH
	// project, bukan cuma floworkos-go. ws biasanya = floworkos-go saat
	// daemon mode → parent = project root → siblings = flowork-kernel,
	// flowork_docktor (yang punya go.mod independent).
	if filepath.Base(ws) == "floworkos-go" {
		projectRoot := filepath.Dir(ws)
		entries, _ := os.ReadDir(projectRoot)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(projectRoot, e.Name())
			if candidate == ws {
				continue
			}
			// AddRoot auto-skip kalau ngga ada go.mod (idempotent).
			codemapIndexer.AddRoot(candidate)
		}
	}
	return codemapIndexer, nil
}

func replaceSlash(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '\\' {
			out = append(out, '_', '_')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func readFileOrEmpty(path string) ([]byte, error) {
	return os.ReadFile(path)
}
