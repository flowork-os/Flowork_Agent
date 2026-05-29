// Package tools — worker_sync.go: T3+T4 T-track 2026-04-29.
//
// Saat kernel boot, fetch worker tool list dari floworkos `:3102/tool/list`.
// Untuk tiap worker tool:
//   1. Kalau kernel registry HAS duplicate (e.g. kernel `hak_warga.roadmap_write`
//      vs worker `roadmap_write` after namespace strip), REPLACE kernel version
//      dengan worker stub. LLM lihat worker schema (lengkap dengan period,
//      task, dll).
//   2. Kalau worker-only tool (mis. browser_navigate ngga ada di kernel),
//      register sebagai NEW stub. LLM punya akses tool baru tanpa restart.
//
// Stub tool: schema dari worker, Run() forward ke worker via runViaWorker.
//
// Result: kernel registry jadi superset = kernel-only tools (mesh, karma,
// system) + worker tools (semua 114). LLM lihat semua, schema benar.
//
// Capability gate stays kernel-side (Tier 1 sakral). Worker stubs tetap
// di-protect via capability map.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/flowork/kernel/kernel/types"
)

// workerToolDef mirror dari floworkos provider.ToolDefinition.
type workerToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// workerToolListResponse from /tool/list.
type workerToolListResponse struct {
	Tools []workerToolDef `json:"tools"`
	Count int             `json:"count"`
}

var (
	workerSyncMu       sync.Mutex
	workerSyncDone     bool
	workerSyncedTools  []string // nama tools yang ke-stub
)

// SyncFromWorker fetch worker tool list, register stub di kernel registry.
//
// Idempotent: setiap call check apa yang sudah di-stub, hanya register baru.
// Caller (cmd/kernel/main.go) call ini setelah boot complete.
//
// Return jumlah tool yang ke-register/ke-replace, atau error kalau worker
// unreachable.
func SyncFromWorker(ctx context.Context) (int, error) {
	workerSyncMu.Lock()
	defer workerSyncMu.Unlock()

	workerURL := resolveWorkerURL()
	if workerURL == "" {
		return 0, fmt.Errorf("worker URL not configured")
	}
	token := resolveWorkerToken()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, workerURL+"/tool/list", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch worker tool list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("worker /tool/list HTTP %d: %s", resp.StatusCode, body)
	}

	var listResp workerToolListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return 0, fmt.Errorf("parse worker tool list: %w", err)
	}

	// Build kernel candidate match index. Untuk satu kernel tool "X.Y", kita
	// generate 2 alias yang mungkin overlap dengan worker tool name:
	//   - simple "Y" (last segment, mis. "filesystem.read" -> "read")
	//   - prefix-joined "X_Y" (mis. "brain.search" -> "brain_search")
	// existingNames bisa multi-value per key (kernel mungkin punya 2 tools
	// yang berbeda namespace tapi simple name sama, e.g. cron.list dan
	// filesystem.list). Sehingga value = []string.
	existingNames := make(map[string][]string)
	for _, name := range listToolsRegistry() {
		simple := stripKernelNamespace(name)
		existingNames[simple] = append(existingNames[simple], name)
		if dot := strings.Index(name, "."); dot > 0 {
			joined := strings.ReplaceAll(name, ".", "_")
			existingNames[joined] = append(existingNames[joined], name)
		}
	}

	registered := 0
	replaced := 0
	deduped := 0
	for _, wt := range listResp.Tools {
		stub := &workerStubTool{
			name:        wt.Name,
			description: wt.Description,
			schema:      wt.InputSchema,
		}

		// T4 T-track 2026-04-29 (Tier slim): worker = canonical executor.
		// Kalau kernel punya tool yang strip/join-nya match worker name,
		// UNREGISTER semua kernel duplicate. Kernel = pure pemancar.
		matches := existingNames[wt.Name]
		hasNamespacedDup := false
		for _, m := range matches {
			if m != wt.Name {
				unregisterTool(m)
				deduped++
				hasNamespacedDup = true
			}
		}
		if hasNamespacedDup {
			if _, err := registerOrReplace(stub); err != nil {
				log.Printf("[tools.worker_sync] register %s skip: %v", wt.Name, err)
				continue
			}
			replaced++
		} else if _, exists := getRegistry()[wt.Name]; exists {
			// Same exact name — replace with worker stub
			if _, err := registerOrReplace(stub); err != nil {
				log.Printf("[tools.worker_sync] replace %s skip: %v", wt.Name, err)
				continue
			}
			replaced++
		} else {
			// New tool, just register
			if _, err := registerOrReplace(stub); err != nil {
				log.Printf("[tools.worker_sync] register %s skip: %v", wt.Name, err)
				continue
			}
			registered++
		}
		workerSyncedTools = append(workerSyncedTools, wt.Name)
	}

	workerSyncDone = true
	log.Printf("[tools.worker_sync] %d new + %d replaced + %d kernel duplicates removed (%s)", registered, replaced, deduped, workerURL)
	return registered + replaced, nil
}

// unregisterTool remove tool dari registry by name. T4 dedup support.
func unregisterTool(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, name)
}

// workerStubTool implements types.Tool — proxy ke worker via runViaWorker.
type workerStubTool struct {
	name        string
	description string
	schema      map[string]any
}

var _ types.Tool = (*workerStubTool)(nil)

func (t *workerStubTool) Name() string                  { return t.name }
func (t *workerStubTool) Description() string           { return t.description }
func (t *workerStubTool) InputSchema() map[string]any   { return t.schema }
func (t *workerStubTool) Run(ctx context.Context, args map[string]any) (any, error) {
	// Worker stub bypass kernel namespace strip — name udah simple.
	result, err := runViaWorker(ctx, t.name, args)
	if err != nil {
		return nil, fmt.Errorf("worker stub %s: %w", t.name, err)
	}
	return result, nil
}

// listToolsRegistry return all kernel-registered tool names.
// (Wrapper supaya ngga expose internal map directly.)
func listToolsRegistry() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}

// getRegistry helper untuk akses internal map.
func getRegistry() map[string]types.Tool {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]types.Tool, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// registerOrReplace force-register tool, replacing existing entry kalau ada.
// Test kemudian Run via worker via runViaWorker.
func registerOrReplace(t types.Tool) (bool, error) {
	mu.Lock()
	defer mu.Unlock()
	name := t.Name()
	if name == "" {
		return false, fmt.Errorf("empty tool name")
	}
	registry[name] = t
	// Add to capability map kalau belum ada (default = same as name).
	// Wildcard "*" warga bypass map anyway; ini supaya non-wildcard warga
	// bisa di-grant tool baru secara explicit di future.
	if _, ok := capabilityMap[name]; !ok {
		capabilityMap[name] = name
	}
	return true, nil
}

// IsWorkerSynced return true kalau SyncFromWorker pernah sukses.
func IsWorkerSynced() bool {
	workerSyncMu.Lock()
	defer workerSyncMu.Unlock()
	return workerSyncDone
}

// WasWorkerSynced cek apakah tool name sebelumnya pernah di-sync dari worker.
// Pakai snapshot historic terakhir kali sync sukses — bukan reflect state
// registry sekarang.
//
// Use case (keluh-kesah #322/#325): kalau ErrToolNotFound terjadi tapi
// tool name pernah ke-sync (= worker tools, bukan kernel-native), itu
// signal worker daemon kemungkinan baru mati. Caller upgrade error ke
// ErrWorkerOffline untuk error message yang lebih jelas.
func WasWorkerSynced(name string) bool {
	workerSyncMu.Lock()
	defer workerSyncMu.Unlock()
	for _, n := range workerSyncedTools {
		if n == name {
			return true
		}
	}
	return false
}

// WorkerSyncedTools return list of tool names yang ke-sync dari worker.
func WorkerSyncedTools() []string {
	workerSyncMu.Lock()
	defer workerSyncMu.Unlock()
	out := make([]string, len(workerSyncedTools))
	copy(out, workerSyncedTools)
	return out
}

// stub helper untuk format output
var _ = strings.ToLower
