// === LOCKED FILE (soft) — STABLE (ROADMAP 4 v1, owner-approved 2026-06-07). ===
// 2026-06-07: ensureProc spawn-lock (fix double-spawn race, owner-mandated audit).
// Substrat platform; app = plugin di apps/<id>/ (JANGAN edit substrat utk app baru).
//
// Package apps — ROADMAP 4: platform aplikasi dipakai-bersama MANUSIA & AGENT.
//
// Sebuah APP = core headless (state + operasi) + GUI + manifest. Kuncinya: "satu state, dua
// pengemudi" — manusia lewat GUI, agent lewat TOOL, keduanya memanggil operasi yang SAMA via
// invokeOp. Core LINTAS BAHASA (runtime:process → bahasa apa pun via stdio JSON, lihat proc.go).
// Inti tak tahu logika app; tipe app = plugin di folder apps/<id>/.
package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/tools"
)

// Op — satu operasi app (jadi tombol GUI DAN tool agent).
type Op struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Tool        bool           `json:"tool"` // expose sebagai tool agent
	GUI         bool           `json:"gui"`
	Mutates     bool           `json:"mutates"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// Manifest — manifest.json app.
type Manifest struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Version     string `json:"version"`
	Runtime     string `json:"runtime"`    // "process" (v1) | wasm/http (future)
	CoreEntry   string `json:"core_entry"` // mis. "python3 core.py"
	GUIEntry    string `json:"gui_entry"`  // mis. "ui/index.html"
	Operations  []Op   `json:"operations"`
}

// App — manifest + folder.
type App struct {
	Manifest
	Dir string `json:"-"`
}

func (a *App) op(name string) (Op, bool) {
	for _, o := range a.Operations {
		if o.Name == name {
			return o, true
		}
	}
	return Op{}, false
}

// Manager — registri app + core yang jalan + tool yang terdaftar. Aman concurrent.
type Manager struct {
	mu      sync.Mutex
	spawnMu sync.Mutex // serialize spawn core (anti double-spawn race, lihat ensureProc)
	dir     string
	apps    map[string]*App
	procs   map[string]*proc
	regs    map[string][]string
	version map[string]int64 // state_version per app (untuk sinkron GUI)
}

func NewManager(appsDir string) *Manager {
	return &Manager{dir: appsDir, apps: map[string]*App{}, procs: map[string]*proc{}, regs: map[string][]string{}, version: map[string]int64{}}
}

var appIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)

// Load — scan folder apps/, baca manifest kind:app, daftarkan operasi sebagai tool agent.
func (m *Manager) Load() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil // belum ada folder apps → tak apa
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(m.dir, e.Name(), "manifest.json"))
		if rerr != nil {
			continue
		}
		var man Manifest
		if json.Unmarshal(raw, &man) != nil || man.Kind != "app" || !appIDRe.MatchString(man.ID) {
			continue
		}
		app := &App{Manifest: man, Dir: filepath.Join(m.dir, e.Name())}
		m.mu.Lock()
		m.apps[man.ID] = app
		m.mu.Unlock()
		m.registerTools(app)
	}
	return nil
}

// List — semua app terpasang.
func (m *Manager) List() []*App {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*App, 0, len(m.apps))
	for _, a := range m.apps {
		out = append(out, a)
	}
	return out
}

// Get — app by id.
func (m *Manager) Get(id string) (*App, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.apps[id]
	return a, ok
}

// ensureProc — spawn core kalau belum jalan (lazy). Caller TIDAK pegang m.mu.
func (m *Manager) ensureProc(app *App) (*proc, error) {
	m.mu.Lock()
	if p, ok := m.procs[app.ID]; ok {
		m.mu.Unlock()
		return p, nil
	}
	m.mu.Unlock()
	if app.Runtime != "process" && app.Runtime != "" {
		return nil, fmt.Errorf("runtime %q belum didukung (v1: process)", app.Runtime)
	}
	// spawn-lock: tanpa ini dua caller (GUI manusia + tool agent) yang nembak app
	// SAMA pertama kali bisa lolos cek di atas berbarengan → double-spawn → proc
	// kedua nimpa yang pertama (zombie) + state pecah. Re-check di dalam lock.
	m.spawnMu.Lock()
	defer m.spawnMu.Unlock()
	m.mu.Lock()
	if p, ok := m.procs[app.ID]; ok {
		m.mu.Unlock()
		return p, nil
	}
	m.mu.Unlock()
	p, err := startProc(app.CoreEntry, app.Dir)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.procs[app.ID] = p
	m.mu.Unlock()
	return p, nil
}

// coreResp — bentuk balasan core.
type coreResp struct {
	Result       json.RawMessage `json:"result"`
	StateVersion int64           `json:"state_version"`
	Error        string          `json:"error"`
}

// InvokeOp — JANTUNG: SATU pintu untuk DUA pengemudi (human GUI & agent tool). Validasi op →
// panggil core → balik result. caller = "human-gui" | "agent:<id>".
func (m *Manager) InvokeOp(id, op string, args map[string]any, caller string) (any, error) {
	app, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("app tak ditemukan: %s", id)
	}
	spec, ok := app.op(op)
	if !ok {
		return nil, fmt.Errorf("operasi tak terdaftar: %s", op)
	}
	p, err := m.ensureProc(app)
	if err != nil {
		return nil, err
	}
	argsJSON, _ := json.Marshal(args)
	line, err := p.call(op, argsJSON, 120*time.Second)
	if err != nil {
		// core mati → buang biar di-restart next call
		m.mu.Lock()
		if pp := m.procs[id]; pp == p {
			pp.close()
			delete(m.procs, id)
		}
		m.mu.Unlock()
		return nil, err
	}
	var resp coreResp
	if json.Unmarshal(line, &resp) != nil {
		return nil, fmt.Errorf("core balas non-JSON")
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("app: %s", resp.Error)
	}
	if spec.Mutates {
		m.mu.Lock()
		m.version[id]++
		m.mu.Unlock()
	}
	var result any
	_ = json.Unmarshal(resp.Result, &result)
	return result, nil
}

// StateVersion — versi state app (GUI poll untuk sinkron human↔agent).
func (m *Manager) StateVersion(id string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version[id]
}

// Shutdown — matikan semua core.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.procs {
		p.close()
	}
	m.procs = map[string]*proc{}
}

// Stop — matikan core SATU app (kalau jalan); state di memori-nya hilang dan op
// berikutnya lazy-spawn ulang lewat ensureProc. Owner-approved 2026-06-11: dipakai
// saat tab app DITUTUP di GUI (browser-tab shell) supaya app mati ketika gak ada
// tab kebuka — proses cuma hidup selama tab-nya ada. No-op kalau app gak jalan.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[id]; ok {
		p.close()
		delete(m.procs, id)
	}
}

// ── op → tool bridge (sisi AGENT) ────────────────────────────────────────────

var nonTool = regexp.MustCompile(`[^a-z0-9_]+`)

func toolName(appID, op string) string {
	return nonTool.ReplaceAllString(strings.ToLower("app_"+appID+"_"+op), "_")
}

// registerTools — daftarkan tiap operasi tool:true sebagai tool agent (reuse tools.RegisterDynamic,
// pola sama mcphub.bridgeTool). Agent menemukan & memakai operasi app lewat tool_search/tools/run.
func (m *Manager) registerTools(app *App) {
	regs := []string{}
	for _, o := range app.Operations {
		if !o.Tool {
			continue
		}
		bt := &appTool{mgr: m, appID: app.ID, op: o.Name, appName: app.Name, spec: o}
		if err := tools.RegisterDynamic(bt); err != nil {
			continue // bentrok nama → skip 1 tool, sisanya jalan
		}
		regs = append(regs, bt.Name())
	}
	m.mu.Lock()
	m.regs[app.ID] = regs
	m.mu.Unlock()
}

// appTool — bridge satu operasi app jadi tools.Tool.
type appTool struct {
	mgr     *Manager
	appID   string
	op      string
	appName string
	spec    Op
}

func (t *appTool) Name() string       { return toolName(t.appID, t.op) }
func (t *appTool) Capability() string { return "app:" + t.appID }
func (t *appTool) Schema() tools.Schema {
	desc := t.spec.Description
	if desc == "" {
		desc = t.op + " (app " + t.appName + ")"
	}
	keys := []string{}
	if props, ok := t.spec.InputSchema["properties"].(map[string]any); ok {
		for k := range props {
			keys = append(keys, k)
		}
	}
	if len(keys) > 0 {
		desc += " — args: {" + strings.Join(keys, ", ") + "}"
	}
	return tools.Schema{Description: "[App: " + t.appName + "] " + desc, Returns: "operation result (JSON)"}
}
func (t *appTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	out, err := t.mgr.InvokeOp(t.appID, t.op, args, "agent")
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: out}, nil
}
