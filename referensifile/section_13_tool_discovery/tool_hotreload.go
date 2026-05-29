// Package tools — tool_hotreload.go: Tier 1.1 Evolusi.
//
// Per Ayah arahan 2026-05-17: evolusi workspace-level + tier 1 soft tools.
// Pattern adopt Claude Code AgentToolDefinition pattern (file-based dropable
// .json di skill-style folder).
//
// Tool registry hot-reload: warga AI bisa propose tool baru via tool
// "tool_propose" → write JSON spec ke `bundled/tools/proposed/<name>.json` →
// Ayah review approve (move ke `bundled/tools/active/`) → kernel auto-reload
// pada tick berikutnya (TTL cache 5 menit).
//
// Tool spec format (JSON, simple — no code execution):
//   {
//     "name": "my_helper",
//     "description": "1-line summary",
//     "category": "research|coding|comm|...",
//     "schema": {...},                  // JSON Schema input
//     "impl": {
//       "type": "alias",                // alias | composite | external
//       "target": "web_search",         // alias to existing tool
//       "args_template": {...}          // pre-fill args
//     }
//   }
//
// Impl types:
//   - "alias":     wrap existing tool dengan default args (e.g. web_search dengan site filter)
//   - "composite": chain 2-3 existing tools (sequential exec, output->input)
//   - "external":  EXEC dilarang dari hot-reload (security boundary —
//                  external tools harus via PR + Ayah review + rebuild)
//
// Safety guard:
//   - Default location: bundled/tools/active/ (read-only saat runtime)
//   - Proposed: bundled/tools/proposed/ (Ayah review zone, ngga auto-register)
//   - SACRED whitelist: 22 core tool (read, write, edit, bash, web_search,
//     brain_search, dll) NEVER bisa di-override via hot-reload.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
)

// HotReloadToolSpec — JSON file format untuk dropable tool definition.
type HotReloadToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category,omitempty"`
	Schema      map[string]any `json:"schema"`
	Impl        struct {
		Type         string         `json:"type"` // "alias" | "composite"
		Target       string         `json:"target,omitempty"`
		ArgsTemplate map[string]any `json:"args_template,omitempty"`
		Steps        []struct {
			Tool string         `json:"tool"`
			Args map[string]any `json:"args"`
		} `json:"steps,omitempty"`
	} `json:"impl"`
}

// SACRED_TOOLS — core tool yang NEVER bisa di-override via hot-reload.
// Modifikasi cuma via PR + rebuild kernel (high friction safety).
var sacredToolNames = map[string]bool{
	"read": true, "write": true, "edit": true, "multiedit": true,
	"bash": true, "powershell": true, "notebookedit": true,
	"web_search": true, "web_fetch": true, "browser_render": true,
	"brain_search": true, "memorize_brain": true, "brain_post_drawer": true,
	"telegram_send": true, "forum_post": true, "dream_post": true,
	"plan_write": true, "plan_read": true, "memory_set": true,
	"git_checkpoint": true, "git_verify": true, "git_rollback": true,
	"flowork_anti_zombie": true,
}

// HotReloadRegistry — manages dropable tool .json di bundled/tools/active/.
type HotReloadRegistry struct {
	mu       sync.RWMutex
	root     string
	specs    map[string]*HotReloadToolSpec
	lastScan time.Time
	ttl      time.Duration
}

func NewHotReloadRegistry(workspace string) *HotReloadRegistry {
	return &HotReloadRegistry{
		root:  filepath.Join(workspace, "bundled", "tools"),
		specs: map[string]*HotReloadToolSpec{},
		ttl:   5 * time.Minute,
	}
}

// ScanActive walks bundled/tools/active/*.json dan refresh cache.
func (r *HotReloadRegistry) ScanActive() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	activeDir := filepath.Join(r.root, "active")
	if _, err := os.Stat(activeDir); err != nil {
		// Direktori belum ada — buat empty supaya warga tau struktur.
		_ = os.MkdirAll(activeDir, 0755)
		_ = os.MkdirAll(filepath.Join(r.root, "proposed"), 0755)
		r.lastScan = time.Now()
		return nil
	}

	newSpecs := map[string]*HotReloadToolSpec{}
	entries, err := os.ReadDir(activeDir)
	if err != nil {
		return fmt.Errorf("scan active dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(activeDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var spec HotReloadToolSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			continue
		}
		// Reject SACRED override
		if sacredToolNames[spec.Name] {
			continue
		}
		// Reject empty / invalid
		if spec.Name == "" || spec.Description == "" || spec.Impl.Type == "" {
			continue
		}
		// Reject "external" impl type (security boundary)
		if spec.Impl.Type == "external" {
			continue
		}
		newSpecs[spec.Name] = &spec
	}
	r.specs = newSpecs
	r.lastScan = time.Now()
	return nil
}

// MaybeRescan re-scan kalau TTL expired.
func (r *HotReloadRegistry) MaybeRescan() {
	r.mu.RLock()
	stale := time.Since(r.lastScan) > r.ttl
	r.mu.RUnlock()
	if stale {
		_ = r.ScanActive()
	}
}

// List returns active hot-reload tool specs (untuk register ke main registry).
func (r *HotReloadRegistry) List() []*HotReloadToolSpec {
	r.MaybeRescan()
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*HotReloadToolSpec, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, s)
	}
	return out
}

// ─── HotReloadProposeTool — warga AI write spec ke bundled/tools/proposed/ ───
// Ayah review manual lalu move ke active/ untuk approve.

type HotReloadProposeTool struct {
	registry *HotReloadRegistry
}

type hotReloadProposeArgs struct {
	Spec string `json:"spec" validate:"required"` // JSON-serialized HotReloadToolSpec
}

func NewHotReloadProposeTool(reg *HotReloadRegistry) *HotReloadProposeTool {
	return &HotReloadProposeTool{registry: reg}
}

func (t *HotReloadProposeTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "tool_propose_hotreload",
		Description: "Propose tool baru (Tier 1.1 hot-reload). Drop JSON spec " +
			"ke bundled/tools/proposed/<name>.json — Ayah review approve " +
			"dengan move ke bundled/tools/active/. Sacred tool ngga bisa override.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spec": map[string]any{
					"type":        "string",
					"description": "JSON-serialized HotReloadToolSpec (lihat tool_hotreload.go untuk format)",
				},
			},
			"required": []string{"spec"},
		},
	}
}

func (t *HotReloadProposeTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args hotReloadProposeArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("tool_propose_hotreload: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("tool_propose_hotreload: validation: %w", err)
	}

	var spec HotReloadToolSpec
	if err := json.Unmarshal([]byte(args.Spec), &spec); err != nil {
		return Result{}, fmt.Errorf("tool_propose_hotreload: spec invalid JSON: %w", err)
	}
	if spec.Name == "" || spec.Description == "" || spec.Impl.Type == "" {
		return Result{}, fmt.Errorf("tool_propose_hotreload: name + description + impl.type WAJIB diisi")
	}
	if sacredToolNames[spec.Name] {
		return Result{}, fmt.Errorf("tool_propose_hotreload: %q is SACRED tool — ngga bisa override via hot-reload (PR + rebuild required)", spec.Name)
	}
	if spec.Impl.Type == "external" {
		return Result{}, fmt.Errorf("tool_propose_hotreload: impl.type=external blocked (security boundary, butuh PR)")
	}
	if spec.Impl.Type != "alias" && spec.Impl.Type != "composite" {
		return Result{}, fmt.Errorf("tool_propose_hotreload: impl.type harus 'alias' atau 'composite', got %q", spec.Impl.Type)
	}

	proposedDir := filepath.Join(t.registry.root, "proposed")
	if err := os.MkdirAll(proposedDir, 0755); err != nil {
		return Result{}, fmt.Errorf("tool_propose_hotreload: mkdir proposed: %w", err)
	}

	// Sanitize filename
	safeName := strings.ReplaceAll(spec.Name, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	dst := filepath.Join(proposedDir, safeName+".json")

	out, _ := json.MarshalIndent(spec, "", "  ")
	if err := os.WriteFile(dst, out, 0644); err != nil {
		return Result{}, fmt.Errorf("tool_propose_hotreload: write: %w", err)
	}

	return Result{
		Output: fmt.Sprintf("# Tool Proposed\n\n"+
			"**Name:** %s\n**Type:** %s\n**Target:** %s\n\n"+
			"File: `bundled/tools/proposed/%s.json`\n\n"+
			"**Approve flow (Ayah):**\n"+
			"  mv bundled/tools/proposed/%s.json bundled/tools/active/\n\n"+
			"Kernel akan auto-reload pada tick berikutnya (TTL 5 menit).",
			spec.Name, spec.Impl.Type, spec.Impl.Target, safeName, safeName),
		Metadata: map[string]any{
			"name":    spec.Name,
			"path":    dst,
			"approved": false,
		},
	}, nil
}
