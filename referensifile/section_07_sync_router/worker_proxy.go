// Package tools — worker_proxy.go: T5 T-track 2026-04-29.
//
// Per arahan Ayah: kernel = pemancar (Tier 1 sakral), tools execution di
// floworkos worker (Tier 2/3). Setelah capability gate kernel lulus, tool
// call HTTP-forward ke worker `:3102/exec/<tool>`.
//
// Worker URL resolution order:
//  1. Settings DB key `FLOWORK_WORKER_URL`
//  2. Env `FLOWORK_WORKER_URL`
//  3. Empty = fallback ke local kernel registry (legacy mode)
//
// Auth: Bearer FLOWORK_KERNEL_TOKEN (sama dengan kernel auth — single token).

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flowork/kernel/kernel/settings"
)

const (
	workerSettingsKey = "FLOWORK_WORKER_URL"
	workerTimeout     = 5 * time.Minute
)

// workerProxyClient — singleton (hunting_bug 2026-04-30 BUG-008 fix).
// Sebelumnya bikin http.Client baru per call → no TCP connection reuse,
// FD exhaustion potential. Singleton reuse internal transport pool.
var workerProxyClient = &http.Client{Timeout: workerTimeout}

// resolveWorkerURL get worker base URL from env > settings DB > default.
// Empty string = proxy disabled, fallback ke local kernel registry.
func resolveWorkerURL() string {
	// 1. Env (paling reliable, set saat kernel boot)
	if v := strings.TrimSpace(os.Getenv(workerSettingsKey)); v != "" {
		return strings.TrimRight(v, "/")
	}
	// 2. Settings DB
	if store := settings.Shared(); store != nil {
		if v, _ := store.Get(workerSettingsKey); strings.TrimSpace(v) != "" {
			return strings.TrimRight(v, "/")
		}
	}
	// 3. Default: localhost:3102 (Tier 2/3 worker default port)
	return "http://localhost:3102"
}

// resolveWorkerToken get auth token for kernel→worker calls.
//
// Sprint 3.5e (BUG-C24 fix 2026-05-02): priority chain reordered + state file.
// OLD chain: (1) settings DB → (2) env → (3) "BapakPembangunan" hardcode.
// Problem: on first boot settings DB empty, env not always set, so kernel
// sent "BapakPembangunan" to worker → worker expected real token → 403 →
// SyncFromWorker permanent failure → Merpati stuck with 8 tools.
//
// NEW chain: (1) env FLOWORK_KERNEL_TOKEN → (2) settings DB → (3) state file.
// State file is written by kernel middleware_auth.go at boot.
//
// NOTE untuk AI selanjutnya: ZERO HARDCODE token. Kalau semua 3 gagal,
// return empty string → caller gets 401, log will show why.
func resolveWorkerToken() string {
	// Tier 1: env var (set by launch-kernel.bat from state file)
	if v := strings.TrimSpace(os.Getenv("FLOWORK_KERNEL_TOKEN")); v != "" {
		return v
	}
	// Tier 2: settings DB
	if store := settings.Shared(); store != nil {
		if v, _ := store.Get("KERNEL_API_KEY"); strings.TrimSpace(v) != "" {
			return v
		}
	}
	// Tier 3: read state file directly (fallback for cases where env not set)
	if data, err := os.ReadFile(resolveStateFilePath()); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v
		}
	}
	return "" // no fallback — caller gets 401, operator must check token
}

// resolveStateFilePath returns the project root state/kernel/auth_token path.
// Uses same logic as middleware_auth.go exportTokenToStateFile.
func resolveStateFilePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// kernel.exe is in flowork-kernel/ → project root is parent
	kernelDir := filepath.Dir(exe)
	projectRoot := filepath.Dir(kernelDir)
	return filepath.Join(projectRoot, "state", "kernel", "auth_token")
}

// workerExecRequest payload sent ke worker /exec/<tool>.
type workerExecRequest struct {
	Arguments  json.RawMessage `json:"arguments"`
	SessionID  string          `json:"session_id"`
	WorkingDir string          `json:"working_dir"`
	CallID     string          `json:"call_id"`
}

// workerExecResponse mirror dari floworkos-go/internal/tools.Result.
type workerExecResponse struct {
	ToolName string                 `json:"tool_name"`
	OK       bool                   `json:"ok"`
	Output   string                 `json:"output"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// stripKernelNamespace map kernel tool name (e.g. "hak_warga.roadmap_write")
// ke floworkos worker name (e.g. "roadmap_write"). Worker tools register
// tanpa namespace prefix; kernel pakai prefix untuk organize.
//
// Drop everything before LAST dot. "hak_warga.roadmap_write" -> "roadmap_write"
// "filesystem.write" -> "write" (caller harus aware).
//
// Special-case: nama tanpa dot tetap as-is.
func stripKernelNamespace(toolName string) string {
	if idx := strings.LastIndex(toolName, "."); idx >= 0 && idx < len(toolName)-1 {
		return toolName[idx+1:]
	}
	return toolName
}

// runViaWorker HTTP-forward tool execution ke floworkos worker.
//
// Return raw output string + error. Caller (Run di registry.go) decide gimana
// wrap result.
func runViaWorker(ctx context.Context, toolName string, args map[string]any) (any, error) {
	workerURL := resolveWorkerURL()
	if workerURL == "" {
		return nil, errWorkerNotConfigured
	}
	token := resolveWorkerToken()
	// Strip kernel namespace prefix: hak_warga.roadmap_write -> roadmap_write
	// Sprint 3.5g 2026-05-03: ALSO resolve canonical alias (multi_edit ->
	// multiedit, sandbox -> run_sandbox_command, dst). Lihat aliases.go —
	// reconcile GUI cap toggle vs worker registered Definition().Name.
	workerToolName := ResolveAlias(stripKernelNamespace(toolName))

	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("worker proxy: marshal args: %w", err)
	}
	body := workerExecRequest{
		Arguments: argsJSON,
		// SessionID + WorkingDir + CallID di-fill dari ctx future enhancement.
	}
	bodyJSON, _ := json.Marshal(body)

	url := workerURL + "/exec/" + workerToolName
	log.Printf("[tools.worker_proxy] forwarding %s -> %s", toolName, url)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("worker proxy: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// X-Flowork-Agent: extract warga identity dari args[agent] / args[from]
	// (di-inject process_message.go line 274/830). Worker's exec handler baca
	// header ini supaya karma check resolve per-request, BUKAN env var single-
	// process. Fix per Ayah QC 2026-05-11.
	if v, ok := args["agent"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			req.Header.Set("X-Flowork-Agent", strings.TrimSpace(s))
		}
	} else if v, ok := args["from"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			req.Header.Set("X-Flowork-Agent", strings.TrimSpace(s))
		}
	}

	resp, err := workerProxyClient.Do(req)
	if err != nil {
		// 2026-05-07 (keluh-kesah #322/#325): connection refused / dial fail =
		// worker daemon mati. Sebelumnya wrapped pake fmt.Errorf jadi
		// run_error generic 500. Sekarang detect dial-level error +
		// upgrade ke ErrWorkerOffline supaya route_tool_execute return
		// 503 worker_offline dengan pesan jelas.
		if isWorkerOfflineError(err) {
			return nil, fmt.Errorf("%w: dispatch %s ke %s gagal: %v", ErrWorkerOffline, toolName, url, err)
		}
		return nil, fmt.Errorf("worker proxy: call %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("worker proxy: %s returned HTTP %d: %s",
			workerToolName, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out workerExecResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("worker proxy: parse response: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("worker tool error: %s", out.Output)
	}
	// Return Output sebagai string + Metadata. Caller cast type.
	return out.Output, nil
}

// errWorkerNotConfigured raised kalau worker URL kosong (proxy disabled).
// Caller fallback ke local registry.
var errWorkerNotConfigured = fmt.Errorf("worker not configured (set FLOWORK_WORKER_URL)")

// isWorkerOfflineError cek apakah error dari http.Client.Do() adalah
// indikasi worker daemon offline (connection refused, no route to host,
// dll) — bukan business error dari worker side.
//
// Kenapa: keluh-kesah #322/#325 — kalau worker port 3102 ngga listening,
// dial fail dengan error yang containt "connection refused" / "actively
// refused" / "no connection could be made". Ini sinyal kuat worker daemon
// mati, bukan tool error. Caller upgrade ke ErrWorkerOffline.
func isWorkerOfflineError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "actively refused"),
		strings.Contains(msg, "no connection could be made"),
		strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "connect: connection reset"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "dial tcp"):
		return true
	}
	return false
}
