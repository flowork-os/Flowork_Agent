// Package tools — registry untuk semua tool kernel.
//
// 4 prinsip absolut:
//   1. Nano modular — registry.go cuma lifecycle, tool implementation di file/folder lain
//   2. Plug-and-play — tool register diri via init(), ngga edit registry
//   3. Portable — pure Go, no platform-specific
//   4. Multi-OS — interface abstract OS detail
//
// Pattern tool registration (lihat kernel/tools/git/checkpoint.go contoh):
//
//	package git
//	func init() { tools.Register(&checkpointTool{}) }
//	type checkpointTool struct{}
//	func (t *checkpointTool) Name() string { return "git.checkpoint" }
//	... (implement types.Tool interface)
//
// Tambah tool baru = bikin file di kernel/tools/<category>/<name>.go.
// Tambah category baru = bikin folder + import blank di cmd/kernel/main.go.

package tools

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/flowork/kernel/kernel/safety"
	"github.com/flowork/kernel/kernel/types"
)

// Sentinel errors.
var (
	ErrToolNotFound  = errors.New("tools: tool not found")
	ErrPermDenied    = errors.New("tools: capability denied for warga")
	ErrInvalidArgs   = errors.New("tools: invalid args")

	// ErrWorkerOffline — keluh-kesah #322/#325 merpati 2026-05-04: kalau
	// worker daemon di port 3102 mati, tools_count drop dari 130 ke 9
	// (cuma kernel-native tools survive). Pre-fix kernel return
	// ErrToolNotFound → warga halu "tool ilang permanent". Sekarang
	// distinguished: kalau dispatch fail karena worker offline,
	// return error message yang jelas supaya warga tahu solusinya
	// (restart worker, bukan tool deprecated).
	//
	// Detection strategy: kalau ErrToolNotFound + tool name pernah
	// di-sync dari worker (WasWorkerSynced check), upgrade ke ini.
	ErrWorkerOffline = errors.New("tools: worker daemon offline (port 3102 unreachable, restart flowork-worker)")
)

// registry — process-level singleton tool catalog.
var (
	mu       sync.RWMutex
	registry = make(map[string]types.Tool)
)

// Register tambah tool ke registry. Dipanggil dari init() di setiap tool file.
//
// Plug-and-play: ngga ada manual edit registry.go. Tool baru = bikin file
// dengan init() yang Register(). Idempotent — kalau name duplicate, replace
// (rare case kalau lo override builtin dengan custom).
//
// Panic kalau tool == nil (programmer error, bukan runtime).
func Register(tool types.Tool) {
	if tool == nil {
		panic("tools.Register: nil tool")
	}
	name := tool.Name()
	if name == "" {
		panic("tools.Register: tool.Name() empty")
	}

	mu.Lock()
	defer mu.Unlock()
	registry[name] = tool
}

// Get lookup tool by name. Return ErrToolNotFound kalau ngga ada.
//
// Sprint 3.5g 2026-05-03: kalau direct lookup miss, fallback ke
// ResolveAlias (canonical-name resolution untuk reconcile drift antara
// GUI cap toggle vs registered tool name). Lihat aliases.go.
func Get(name string) (types.Tool, error) {
	mu.RLock()
	t, ok := registry[name]
	mu.RUnlock()
	if ok {
		return t, nil
	}
	// Fallback: alias resolve (e.g. multi_edit -> multiedit)
	canonical := ResolveAlias(name)
	if canonical != name {
		mu.RLock()
		t, ok = registry[canonical]
		mu.RUnlock()
		if ok {
			return t, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrToolNotFound, name)
}

// List return semua tool name registered, sorted alphabetical.
//
// Untuk audit + LLM tool selection ("tools yang lo punya: ..."). Per
// docs/12-CUSTOM-TOOLS.md, list ini di-inject ke warga prompt biar LLM
// ngga halu tool count.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Count return jumlah tool registered.
func Count() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(registry)
}

// Run execute tool dengan capability check. Caller WAJIB lewat fungsi ini
// untuk semua tool execution dari user/warga — JANGAN langsung call
// tool.Run(). Capability gate enforce di sini.
//
// Args:
//   ctx: cancellation + deadline
//   wargaCaps: list capability warga ([]string dari types.Warga.Capabilities())
//             empty list = ngga ada capability sama sekali (default-deny)
//   toolName: nama tool yang mau di-execute
//   args: arguments untuk tool
//
// Return:
//   any: result dari tool
//   error: ErrPermDenied | ErrToolNotFound | ErrInvalidArgs | wrapped run error
//
// Pattern caller (kernel/api/route_tool_execute.go):
//
//	caps := warga.Capabilities()
//	result, err := tools.Run(ctx, caps, "filesystem.read", args)
//	if errors.Is(err, tools.ErrPermDenied) { return 403 }
//	if errors.Is(err, tools.ErrToolNotFound) { return 404 }
//	if err != nil { return 500 }
func Run(ctx context.Context, wargaCaps []string, toolName string, args map[string]any) (any, error) {
	tool, err := Get(toolName)
	if err != nil {
		// Tier 2/3 fallback (2026-05-24): kernel-local registry miss, tapi
		// tool mungkin live di worker :3102 (110 tools). Try worker proxy
		// langsung, skip capability gate (HPG udah enforce di worker side).
		if proxyResult, proxyErr := runViaWorker(ctx, toolName, args); proxyErr == nil {
			return proxyResult, nil
		}
		return nil, err
	}

	// M17 + KEPUTUSAN 1/7: Host Protection Gate (HPG) — hard-coded immutable
	// safety check SEBELUM capability gate. Wildcard "*" admin TIDAK BISA
	// bypass HPG (intentional anti AI rogue). FQP-5 No Wormhole.
	//
	// HPG block dangerous syscall (rm -rf /, format, etc), system path write
	// (/etc, C:\Windows), network target ke loopback/metadata, dan privilege
	// escalation. Audit log via SetCheckHook (kernel boot wire).
	if err := safety.Check(toolName, args); err != nil {
		return nil, err
	}

	// Wildcard "*" admin bypass — workflow + scheduled tasks + test fixtures
	// pakai adminCaps=["*"]. Skip capability map lookup. BUG-034 fix toleran ke
	// tool admin-registered yang ngga perlu di-mapping (mis. wf.test.*).
	if !hasCap(wargaCaps, "*") {
		// 2026-05-01 dual-match capability check (sync dengan
		// kernel/warga/process_message.go buildToolSpecsForWargaAndCaller):
		//
		// PATH 1: GUI per-tool naming — caps berisi flat tool name itself
		// (mis. caps=["brain_search","cron_create",...] dari wargacaps bridge).
		// Granted kalau toolName ada di caps. Ini path utama untuk warga
		// hak-tool toggle GUI.
		//
		// PATH 2 (legacy): capability category mapping. Caps berisi category
		// (mis. caps=["brain","cron","*"]). Tool butuh capReq dari
		// RequiredCapability map. Granted kalau capReq ada di caps.
		//
		// Tanpa dual-match, GUI naming (per-tool) dan legacy naming (category)
		// ngga reconcile → tool ditolak walaupun Ayah centang di GUI.
		if !hasCap(wargaCaps, toolName) {
			// Path 1 fail → fallback Path 2 (legacy category)
			var capReq string
			if dt, ok := tool.(*DynamicTool); ok {
				capReq = dt.Capability
			} else if c, known := RequiredCapability(toolName); known {
				capReq = c
			} else {
				return nil, fmt.Errorf("%w: tool %q not in caps + not in capability map (zero-trust deny)", ErrPermDenied, toolName)
			}

			if !hasCap(wargaCaps, capReq) {
				return nil, fmt.Errorf("%w: warga lacks %q (requires %q OR %q in caps)", ErrPermDenied, toolName, toolName, capReq)
			}
		}
	}



	// Context check (caller dapat cancel di tengah)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Tier 1.5 Foundation 2026-05-10: granular per-arg permission gate.
	// Tool yang implement PermissionAware (opt-in) eval per-call. Tool tanpa
	// implement = default Allow (backward compat, lihat permission.go).
	// HPG + capability gate udah pass — ini layer ketiga finer-grained.
	switch perm := EvalPermission(tool, args); perm.Behavior {
	case PermDeny:
		return nil, fmt.Errorf("%w: tool %q denied per-arg: %s", ErrPermDenied, toolName, perm.Reason)
	case PermAsk:
		// Phase 1 audit-only: log + allow. Phase 2 wire ke gate handler
		// (forum decision / Ayah notification via Telegram).
		log.Printf("[tools.permission] tool=%s ASK: %s (audit-only Phase 1, allowing)", toolName, perm.Reason)
	}

	// T-track 2026-04-29 (T5): try worker proxy first (Tier 2/3 executor di
	// floworkos). Capability gate udah lulus di atas. Worker actual execute.
	// Fallback ke local kernel registry kalau worker disabled/unreachable.
	result, err := runViaWorker(ctx, toolName, args)
	if err == nil {
		return result, nil
	}
	// Worker not configured atau fail → fallback ke local.
	// (Worker error udah ke-format helpful, tapi local fallback jaga uptime.)

	result, err = tool.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("tools.Run %q: %w", toolName, err)
	}
	return result, nil
}

// hasCap cek apakah toolName ada di wargaCaps list. Lowercase = internal.
//
// Default-deny: empty wargaCaps = false untuk semua tool.
// Wildcard "*" = warga punya semua tool (admin tier, hati-hati).
func hasCap(wargaCaps []string, toolName string) bool {
	for _, c := range wargaCaps {
		if c == toolName {
			return true
		}
		if c == "*" {
			return true
		}
	}
	return false
}
