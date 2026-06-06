// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (reversible, owner-editable).
// Owner: Aola Sahidin (Mr.Dev)
// Locked: 2026-06-06 (re-audited 2026-06-07)
// Update 2026-06-07 (owner-approved Connections audit): Enable/Disable/Uninstall now
//   take a per-connector idLock — concurrent Enable(id) (double-click, or boot
//   EnableAll racing a manual enable) previously double-spawned a server and the
//   second store overwrote the first without closing it → orphaned process + tools
//   registered-but-untracked. Per-id lock serializes same-id toggles (diff ids stay
//   parallel). Tested under -race (idlock_test.go).
// Reason: MCP connector hub (ROADMAP_MCP_CONNECTORS.md Phase 2): kind:mcp lifecycle (install/enable/disable/uninstall) + tool bridge (RegisterDynamic per MCP tool → tools.Run→tools/call). Isolated package; owner-gated install. Dogfood-verified via bin/flowork-mcp.
//
// Package mcphub is the registry + lifecycle for "Jenis 2" MCP connectors: external
// MCP servers (github / filesystem / …) that Flowork CONSUMES as tool-sources.
//
// It is deliberately its OWN isolated package (not folded into internal/connections,
// which owns Channels): an MCP connector runs a host process and bridges its tools
// into Flowork's tool registry — a different concern with a different blast radius.
// The two are unified only at the GUI ("Connections" tab → two categories).
//
// How it works (ROADMAP_MCP_CONNECTORS.md):
//   - Install: store {command,args,env} in the connector's OWN folder under
//     ~/.flowork/connectors/mcp/<id>/config.json (self-managed; env holds the token).
//   - Enable: mcpclient.Start spawns the server → tools/list → each tool is
//     tools.RegisterDynamic'd as "mcp_<id>_<tool>" (capability "mcp:<id>"). Agents now
//     reach it through the normal tool system (tool_search → tool.run → SandboxRunV3).
//   - Disable/Uninstall: Unregister the tools + Close (reap) the process.
//
// Owner-approved by design: only the owner installs an MCP server + supplies its
// token. Multi-OS: pure-Go, filepath only.
package mcphub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"flowork-gui/internal/mcpclient"
	"flowork-gui/internal/tools"
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)

var (
	errInvalidID = errors.New("invalid mcp connector id")
	errNotFound  = errors.New("mcp connector not found")
)

// nonToolChar matches anything not allowed in a tool name (keep it verb_noun-ish).
var nonToolChar = regexp.MustCompile(`[^a-z0-9_]+`)

const configFile = "config.json"

// SavedConfig is an MCP server launch spec, stored in the connector's own folder.
type SavedConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Connector is the management view of one MCP connector.
type Connector struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"` // always "mcp"
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	EnvKeys []string `json:"env_keys,omitempty"` // names only — values are secrets
	Enabled bool     `json:"enabled"`            // persistent on/off (no .disabled marker)
	Running bool     `json:"running"`            // currently spawned this process
	Tools   []string `json:"tools,omitempty"`    // registered tool names when running
}

// Manager owns the running MCP servers and the tools they registered. One per
// process (Default). Safe for concurrent use.
type Manager struct {
	mu      sync.Mutex
	servers map[string]*mcpclient.Server
	regs    map[string][]string    // connID -> registered tool names
	locks   map[string]*sync.Mutex // connID -> per-id serialization for Enable/Disable/Uninstall
}

// Default is the process-wide manager.
var Default = &Manager{servers: map[string]*mcpclient.Server{}, regs: map[string][]string{}, locks: map[string]*sync.Mutex{}}

// idLock returns the per-connector lock that serializes Enable/Disable/Uninstall for
// one id. Without it, two concurrent toggles of the SAME connector (a double-click, or
// boot EnableAll racing a manual enable) both pass reap, both Start a server, and the
// second store overwrites the first in m.servers WITHOUT closing it → an orphaned
// process + tools registered-but-untracked. Different ids still proceed in parallel.
func (m *Manager) idLock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks == nil {
		m.locks = map[string]*sync.Mutex{}
	}
	lk := m.locks[id]
	if lk == nil {
		lk = &sync.Mutex{}
		m.locks[id] = lk
	}
	return lk
}

// ── storage (per-connector folder) ───────────────────────────────────────────

func mcpRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".flowork", "connectors", "mcp")
}

func connDir(id string) (string, bool) {
	if !idRe.MatchString(id) {
		return "", false
	}
	return filepath.Join(mcpRoot(), id), true
}

func loadConfig(id string) (SavedConfig, error) {
	dir, ok := connDir(id)
	if !ok {
		return SavedConfig{}, errInvalidID
	}
	raw, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		return SavedConfig{}, errNotFound
	}
	var c SavedConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return SavedConfig{}, err
	}
	return c, nil
}

// Install writes (or replaces) an MCP connector's config in its own folder.
func Install(id string, cfg SavedConfig) error {
	dir, ok := connDir(id)
	if !ok {
		return errInvalidID
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return errors.New("mcp: command required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	blob, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := filepath.Join(dir, configFile+".tmp")
	if err := os.WriteFile(tmp, blob, 0o600); err != nil { // 0600: env may hold a token
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, configFile))
}

// List enumerates installed MCP connectors (running flag + live tool names).
func (m *Manager) List() []Connector {
	out := []Connector{}
	entries, err := os.ReadDir(mcpRoot())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || !idRe.MatchString(e.Name()) {
			continue
		}
		id := e.Name()
		cfg, err := loadConfig(id)
		if err != nil {
			continue
		}
		envKeys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		m.mu.Lock()
		_, running := m.servers[id]
		regs := append([]string(nil), m.regs[id]...)
		m.mu.Unlock()
		out = append(out, Connector{
			ID: id, Kind: "mcp", Command: cfg.Command, Args: cfg.Args,
			EnvKeys: envKeys, Enabled: !isDisabled(id), Running: running, Tools: regs,
		})
	}
	return out
}

const disabledMarker = ".disabled"

// isDisabled reports whether a connector is marked off (persists across restarts).
func isDisabled(id string) bool {
	dir, ok := connDir(id)
	if !ok {
		return true
	}
	_, err := os.Stat(filepath.Join(dir, disabledMarker))
	return err == nil
}

// Enable spawns the MCP server and registers each of its tools into the engine tool
// registry. Re-enabling first disables (clean replace) and clears the off-marker.
func (m *Manager) Enable(ctx context.Context, id string) error {
	lk := m.idLock(id)
	lk.Lock()
	defer lk.Unlock()
	cfg, err := loadConfig(id)
	if err != nil {
		return err
	}
	if dir, ok := connDir(id); ok {
		_ = os.Remove(filepath.Join(dir, disabledMarker)) // mark on (persistent)
	}
	m.reap(id) // idempotent clean slate (no persistent marker)

	srv, err := mcpclient.Start(ctx, id, mcpclient.Config{Command: cfg.Command, Args: cfg.Args, Env: cfg.Env})
	if err != nil {
		return err
	}
	list, err := srv.ListTools(ctx)
	if err != nil {
		_ = srv.Close()
		return err
	}
	regs := make([]string, 0, len(list))
	for _, tl := range list {
		bt := &bridgeTool{
			reg:     toolRegName(id, tl.Name),
			conn:    id,
			mcpName: tl.Name,
			schema:  convertSchema(tl.Description, tl.InputSchema),
			mgr:     m,
		}
		if err := tools.RegisterDynamic(bt); err != nil {
			continue // name clash with a builtin → skip that one tool, keep the rest
		}
		regs = append(regs, bt.reg)
	}
	m.mu.Lock()
	m.servers[id] = srv
	m.regs[id] = regs
	m.mu.Unlock()
	return nil
}

// reap unregisters the connector's tools and stops its process (no persistent
// marker) — the in-memory teardown shared by Disable and a clean re-Enable.
func (m *Manager) reap(id string) error {
	m.mu.Lock()
	srv := m.servers[id]
	regs := m.regs[id]
	delete(m.servers, id)
	delete(m.regs, id)
	m.mu.Unlock()
	for _, r := range regs {
		_ = tools.Unregister(r)
	}
	if srv != nil {
		return srv.Close()
	}
	return nil
}

// Disable reaps the connector AND marks it off persistently (stays off across
// restarts until Enable).
func (m *Manager) Disable(id string) error {
	lk := m.idLock(id)
	lk.Lock()
	defer lk.Unlock()
	err := m.reap(id)
	if dir, ok := connDir(id); ok {
		_ = os.WriteFile(filepath.Join(dir, disabledMarker), []byte("off\n"), 0o644)
	}
	return err
}

// EnableAll starts every installed connector that isn't marked off. Best-effort:
// called at boot; a failing connector is logged by the caller, the rest proceed.
func (m *Manager) EnableAll(ctx context.Context) {
	for _, c := range m.List() {
		if isDisabled(c.ID) {
			continue
		}
		_ = m.Enable(ctx, c.ID)
	}
}

// Uninstall disables then removes the connector's folder (config + token gone).
func (m *Manager) Uninstall(id string) error {
	dir, ok := connDir(id)
	if !ok {
		return errInvalidID
	}
	lk := m.idLock(id)
	lk.Lock()
	defer lk.Unlock()
	_ = m.reap(id)
	return os.RemoveAll(dir)
}

// ToolsFor returns the registered tool names a connector currently exposes (for the
// per-agent checklist).
func (m *Manager) ToolsFor(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.regs[id]...)
}

// toolRegName builds the registry name "mcp_<id>_<tool>" (sanitized to the tool
// charset so it slots into the existing registry cleanly).
func toolRegName(id, mcpTool string) string {
	clean := func(s string) string {
		return strings.Trim(nonToolChar.ReplaceAllString(strings.ToLower(s), "_"), "_")
	}
	return "mcp_" + clean(id) + "_" + clean(mcpTool)
}

// ── tool bridge ──────────────────────────────────────────────────────────────

type bridgeTool struct {
	reg     string
	conn    string
	mcpName string
	schema  tools.Schema
	mgr     *Manager
}

func (t *bridgeTool) Name() string         { return t.reg }
func (t *bridgeTool) Schema() tools.Schema { return t.schema }
func (t *bridgeTool) Capability() string   { return "mcp:" + t.conn }

func (t *bridgeTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	t.mgr.mu.Lock()
	srv := t.mgr.servers[t.conn]
	t.mgr.mu.Unlock()
	if srv == nil {
		return tools.Result{}, errors.New("mcp connector " + t.conn + " is not running")
	}
	text, err := srv.CallTool(ctx, t.mcpName, args)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: text}, nil
}

// convertSchema turns an MCP tool's JSON-Schema input into the engine's tools.Schema.
func convertSchema(desc string, inputSchema json.RawMessage) tools.Schema {
	s := tools.Schema{Description: desc}
	var js struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if json.Unmarshal(inputSchema, &js) != nil {
		return s
	}
	req := map[string]bool{}
	for _, r := range js.Required {
		req[r] = true
	}
	names := make([]string, 0, len(js.Properties))
	for n := range js.Properties {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic order
	for _, n := range names {
		p := js.Properties[n]
		s.Params = append(s.Params, tools.Param{
			Name: n, Type: mapType(p.Type), Description: p.Description, Required: req[n],
		})
	}
	return s
}

func mapType(t string) tools.ParamType {
	switch t {
	case "integer", "number":
		return tools.ParamInt
	case "boolean":
		return tools.ParamBool
	case "array":
		return tools.ParamArray
	case "object":
		return tools.ParamObject
	default:
		return tools.ParamString
	}
}
