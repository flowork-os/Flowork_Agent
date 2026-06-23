// 🔒 FROZEN tools-core — Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// JANGAN edit tanpa unfreeze SADAR owner (di-hash di KERNEL_FREEZE.md + chattr +i). Cara kerja tools,
// cara bikin tool, kenapa SIDECAR, lifecycle, guardrail → WAJIB baca lock/tools.md DULU.
// Mau NAMBAH filtur tanpa buka file ini? Ada CABANG: toolsidecar_ext.go (kebijakan import) — lock/tools.md §switch.

// Package toolsidecar — TOOL PLUG-AND-PLAY ala WordPress lewat SIDECAR proses terpisah.
//
// VISI OWNER (2026-06-23): tiap tool = FOLDER self-contained di `tools/<name>/`:
//   - punya go.mod + dependency SENDIRI (vendor di folder-nya) — NOL shared library.
//   - di-compile jadi BINARY native sendiri (`tools/<name>/<name>`), bukan compiled-in ke kernel.
//   - di-panggil host sbg PROSES TERPISAH (exec) → isolasi (crash/jahat ga nyentuh kernel) + native
//     (lepas dari sandbox WASM) + plug-and-play (drop folder → ke-discover → jalan) + AGNOSTIC.
//
// Kenapa SIDECAR (bukan compiled-in / bukan WASM):
//   - compiled-in privileged tool = ga bisa di-upload (rebuild seluruh binary) + nyatu kernel frozen.
//   - WASM .fwpack = SANDBOX → tool privileged (shell/fs/browser) ga bisa.
//   - SIDECAR = native + modular + isolasi proses → tool "yang beneran kerja" jadi POSTABLE (desktop:
//     drop source → rebuild sidecar → reload). Lihat lock/CognitiveGraph.md? bukan — lihat
//     docs/ROADMAP_MULTI_OS_TOOLS.md §SIDECAR + roadmap_sidecar.md (pola sidecar router).
//
// ABI (kontrak abadi, simpel + paling isolasi): host EXEC binary tool, kirim JSON di STDIN
//
//	{"args": {...}}, tool balikin JSON di STDOUT {"output": <any>, "error": "<str>"}, lalu exit.
//	Stateless per-call (proses fresh tiap panggil) → ga ada state bocor, ga ada port bentrok.
package toolsidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/tools"
)

// Spec — info ringkas tool sidecar (buat GUI + ToolSpecsHandler).
type Spec struct {
	Name        string `json:"name"`
	Capability  string `json:"capability"`
	Description string `json:"description"`
	Params      int    `json:"params"`
	// SELF-EVOLVING (owner 2026-06-23): AgentID = pembuat ("" = shared/bawaan). Scope = "shared"
	// (semua agent) | "private" (cuma pembuat liat+pake, sampai lolos team-review → promote shared).
	AgentID string `json:"agent_id,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

// regSpecs — tool sidecar yg udah ke-register. ToolSpecsHandler baca Names() buat EXPOSE ke SEMUA
// agent (sadar + akses). GUI baca Specs() buat tampil kartu. Owner 2026-06-23.
var (
	regMu    sync.RWMutex
	regSpecs = map[string]Spec{}
)

// Names — daftar nama tool sidecar terdaftar (expose ke spec semua agent). Thread-safe copy.
func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(regSpecs))
	for n := range regSpecs {
		out = append(out, n)
	}
	return out
}

// Specs — info lengkap tiap tool sidecar (buat GUI tab Tools).
func Specs() []Spec {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Spec, 0, len(regSpecs))
	for _, s := range regSpecs {
		out = append(out, s)
	}
	return out
}

// manifest — tools/<name>/tool.json. Schema tool buat di-expose ke LLM.
type manifest struct {
	Name        string          `json:"name"`
	Capability  string          `json:"capability"`
	Description string          `json:"description"`
	Returns     string          `json:"returns"`
	Params      []manifestParam `json:"params"`
}
type manifestParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// sidecarTool — adapter manifest → tools.Tool (di-RegisterDynamic).
type sidecarTool struct {
	m       manifest
	bin     string
	agentID string // "" = shared/bawaan; <id> = privat punya agent itu
	scope   string // "shared" | "private"
}

func (t *sidecarTool) Name() string { return t.m.Name }

// Capability — tool.json kosong → "" (NO gating, BISA DIAKSES SEMUA AGENT, kayak `echo`/`StructuredOutput`).
// Owner 2026-06-23: "tools sidecar bisa diakses semua agent". Tool PRIVILEGED deklarasi cap-nya sendiri
// di tool.json (mis. "exec:foo") → otomatis ke-gate broker (cuma agent ber-cap yg boleh).
func (t *sidecarTool) Capability() string { return strings.TrimSpace(t.m.Capability) }
func (t *sidecarTool) Schema() tools.Schema {
	ps := make([]tools.Param, 0, len(t.m.Params))
	for _, p := range t.m.Params {
		ps = append(ps, tools.Param{Name: p.Name, Type: paramType(p.Type), Description: p.Description, Required: p.Required})
	}
	return tools.Schema{Description: t.m.Description, Params: ps, Returns: t.m.Returns}
}

// Run — EXEC binary tool (proses terpisah), kirim args via stdin, baca output via stdout.
func (t *sidecarTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	reqJSON, _ := json.Marshal(map[string]any{"args": args})
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, t.bin)
	cmd.Stdin = bytes.NewReader(reqJSON)
	// Folder tool jadi CWD → tool bisa baca aset relatif di folder-nya sendiri (self-contained).
	cmd.Dir = filepath.Dir(t.bin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		recordErr(t.m.Name) // GC: error → seleksi alam (rusak/API-berubah → di-prune nanti)
		return tools.Result{}, fmt.Errorf("sidecar %s gagal: %v (%s)", t.m.Name, err, truncate(errb.String(), 200))
	}
	var resp struct {
		Output any    `json:"output"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		recordErr(t.m.Name)
		return tools.Result{}, fmt.Errorf("sidecar %s output ga valid JSON: %v", t.m.Name, err)
	}
	if strings.TrimSpace(resp.Error) != "" {
		recordErr(t.m.Name)
		return tools.Result{}, fmt.Errorf("%s", resp.Error)
	}
	recordUse(t.m.Name) // GC: track last_used → tool nganggur lama = obsolete → di-prune
	return tools.Result{Output: resp.Output}, nil
}

func paramType(s string) tools.ParamType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "int", "integer":
		return tools.ParamInt
	case "float", "number":
		return tools.ParamFloat
	case "bool", "boolean":
		return tools.ParamBool
	default:
		return tools.ParamString
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ToolsDir — folder root sidecar-tools. Override env FLOWORK_TOOLS_DIR (SWITCH), else cari default
// (cwd/tools, ../tools, <exe>/tools) — agnostic lokasi.
func ToolsDir() string {
	if d := strings.TrimSpace(os.Getenv("FLOWORK_TOOLS_DIR")); d != "" {
		return d
	}
	cands := []string{"tools", filepath.Join("..", "tools")}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), "tools"), filepath.Join(filepath.Dir(exe), "..", "tools"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			if abs, e := filepath.Abs(c); e == nil {
				return abs
			}
			return c
		}
	}
	return "tools"
}

// DiscoverAndRegister — scan SHARED (dir/<name>/) + PRIVATE (dir/_private/<agentid>/<name>/) →
// RegisterDynamic tiap yg udah ke-build. Idempoten. Balik (jumlah, nama, unbuilt).
func DiscoverAndRegister(dir string) (registered int, names []string, unbuilt []string) {
	loadHealth(dir) // GC: load error_count/last_used persisted (lintas-restart)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, nil, nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == "_private" {
			// PRIVAT: tools/_private/<agentid>/<name>/ — cuma ke-expose ke pembuatnya.
			agentsDir := filepath.Join(dir, "_private")
			ags, _ := os.ReadDir(agentsDir)
			for _, ag := range ags {
				if !ag.IsDir() {
					continue
				}
				pd := filepath.Join(agentsDir, ag.Name())
				tds, _ := os.ReadDir(pd)
				for _, td := range tds {
					if td.IsDir() && registerOne(filepath.Join(pd, td.Name()), ag.Name(), "private", &names, &unbuilt) {
						registered++
					}
				}
			}
			continue
		}
		if registerOne(filepath.Join(dir, e.Name()), "", "shared", &names, &unbuilt) {
			registered++
		}
	}
	return registered, names, unbuilt
}

// registerOne — baca tool.json + binary → RegisterDynamic + catat scope/owner. Balik true kalau built.
func registerOne(td, agentID, scope string, names, unbuilt *[]string) bool {
	raw, err := os.ReadFile(filepath.Join(td, "tool.json"))
	if err != nil {
		return false // bukan folder tool
	}
	var m manifest
	if json.Unmarshal(raw, &m) != nil || strings.TrimSpace(m.Name) == "" {
		return false
	}
	bin := filepath.Join(td, m.Name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if st, serr := os.Stat(bin); serr != nil || st.IsDir() {
		*unbuilt = append(*unbuilt, m.Name) // source ada, binary belum (jalanin build-tools.sh / tool_create build)
		return false
	}
	_ = tools.RegisterDynamic(&sidecarTool{m: m, bin: bin, agentID: agentID, scope: scope}) // idempoten (dup=ok)
	regMu.Lock()
	regSpecs[m.Name] = Spec{Name: m.Name, Capability: m.Capability, Description: m.Description, Params: len(m.Params), AgentID: agentID, Scope: scope}
	regDir[m.Name] = td
	regMu.Unlock()
	*names = append(*names, m.Name)
	return true
}

// NamesForAgent — tool yang BOLEH dilihat agent ini: SEMUA shared + PRIVAT punya dia sendiri.
// ToolSpecsHandler pakai ini (owner 2026-06-23: privat cuma pembuat yg liat).
func NamesForAgent(agentID string) []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(regSpecs))
	for n, s := range regSpecs {
		if s.Scope != "private" || s.AgentID == agentID {
			out = append(out, n)
		}
	}
	return out
}

// IsPrivate / Owner — buat run-guard (privat cuma boleh dijalanin pembuatnya).
func IsPrivate(name string) bool {
	regMu.RLock()
	defer regMu.RUnlock()
	return regSpecs[name].Scope == "private"
}
func Owner(name string) string {
	regMu.RLock()
	defer regMu.RUnlock()
	return regSpecs[name].AgentID
}

// ── SELF-EVOLVING: agent BIKIN tool privat (owner 2026-06-23) ───────────────
// CreateTool: agent kasih spec + LOGIC (badan fungsi run) → scaffold folder PRIVAT + auto-wrap ABI
// + BUILD-VERIFY (gagal compile → balikin log error buat agent benerin = loop belajar) → register privat.

// CreateParam / CreateSpec — input dari tool_create (lewat agent).
type CreateParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
type CreateSpec struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Capability  string        `json:"capability"`
	Returns     string        `json:"returns"`
	Params      []CreateParam `json:"params"`
	Imports     []string      `json:"imports"`
	Code        string        `json:"code"` // badan: func run(args map[string]any) (any, string) { <CODE> }
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

// CABANG: `dangerImports` (denylist eskalasi native) ada di toolsidecar_ext.go (NON-frozen).
// Mau ubah kebijakan import? edit di sana — JANGAN buka file frozen ini. Detail: lock/tools.md.

// CreateTool — scaffold tools/_private/<agentID>/<name>/ + build + register privat. Balik (buildLog, err).
// err non-nil + buildLog terisi = gagal compile (agent perbaiki code → retry). err non-nil tanpa buildLog
// = error sistem/validasi.
func CreateTool(toolsDir, agentID string, cs CreateSpec) (buildLog string, err error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", fmt.Errorf("agent id ga kebaca (tool wajib ber-owner)")
	}
	name := strings.ToLower(strings.TrimSpace(cs.Name))
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("nama tool invalid (^[a-z][a-z0-9_]{1,39}$): %q", name)
	}
	// Nama = kontrak UNIK global (anti-collision + anti nimpa builtin). Cek registry live.
	if _, exists := tools.Lookup(name); exists {
		return "", fmt.Errorf("nama %q udah dipake tool lain — pilih nama lain (nama = kontrak unik)", name)
	}
	if strings.TrimSpace(cs.Code) == "" {
		return "", fmt.Errorf("code (badan fungsi run) wajib")
	}
	// Guard ringan: tolak import eskalasi native (Fase 1, sebelum sandbox-OS).
	blob := cs.Code + " " + strings.Join(cs.Imports, " ")
	for _, d := range dangerImports {
		if strings.Contains(blob, d) {
			return "", fmt.Errorf("ditolak: pakai %q (eskalasi native) ga diizinin buat tool buatan-agent (sebelum sandbox-OS). Bikin pure-compute dulu", d)
		}
	}

	td := filepath.Join(toolsDir, "_private", agentID, name)
	if err := os.MkdirAll(td, 0o755); err != nil {
		return "", fmt.Errorf("mkdir tool: %w", err)
	}

	// go.mod (modul sendiri — isolasi).
	_ = os.WriteFile(filepath.Join(td, "go.mod"), []byte("module flowork-tool-"+strings.ReplaceAll(name, "_", "-")+"\n\ngo 1.21\n"), 0o644)
	// tool.json (manifest).
	mj, _ := json.MarshalIndent(map[string]any{
		"name": name, "capability": cs.Capability, "description": cs.Description,
		"returns": cs.Returns, "params": cs.Params,
	}, "", "  ")
	_ = os.WriteFile(filepath.Join(td, "tool.json"), mj, 0o644)
	// main.go (auto-wrap ABI + badan run dari agent).
	_ = os.WriteFile(filepath.Join(td, "main.go"), []byte(genMainGo(cs)), 0o644)

	// BUILD-VERIFY (GOWORK=off — isolasi). Gagal → balikin log buat agent benerin.
	cmd := exec.Command("go", "build", "-o", name, ".")
	cmd.Dir = td
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, berr := cmd.CombinedOutput()
	if berr != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("build gagal: %v", berr)
	}

	// Register privat (cuma pembuat yg liat+pake).
	var names, unbuilt []string
	registerOne(td, agentID, "private", &names, &unbuilt)
	return strings.TrimSpace(string(out)), nil
}

// genMainGo — rakit main.go: wrapper ABI tetap + badan run() dari agent + import (dedup base).
func genMainGo(cs CreateSpec) string {
	base := map[string]bool{"encoding/json": true, "io": true, "os": true}
	var extra []string
	for _, imp := range cs.Imports {
		imp = strings.TrimSpace(strings.Trim(imp, `"`))
		if imp != "" && !base[imp] {
			extra = append(extra, "\t\""+imp+"\"")
			base[imp] = true
		}
	}
	imports := "\t\"encoding/json\"\n\t\"io\"\n\t\"os\""
	if len(extra) > 0 {
		imports += "\n" + strings.Join(extra, "\n")
	}
	return "// AUTO-GENERATED by tool_create (sidecar self-evolving). Badan run() dari agent.\n" +
		"package main\n\nimport (\n" + imports + "\n)\n\n" +
		"// run — LOGIC dari agent. args = param dari pemanggil; balikin (output, errString).\n" +
		"func run(args map[string]any) (any, string) {\n" + cs.Code + "\n}\n\n" +
		"func main() {\n" +
		"\tvar req struct {\n\t\tArgs map[string]any `json:\"args\"`\n\t}\n" +
		"\tin, _ := io.ReadAll(os.Stdin)\n\t_ = json.Unmarshal(in, &req)\n" +
		"\tif req.Args == nil {\n\t\treq.Args = map[string]any{}\n\t}\n" +
		"\to, e := run(req.Args)\n" +
		"\tb, _ := json.Marshal(map[string]any{\"output\": o, \"error\": e})\n" +
		"\t_, _ = os.Stdout.Write(b)\n}\n"
}

// ── PROMOTE: private → shared (lewat Dewan, owner 2026-06-23) ────────────────
// regDir — path folder tiap tool (buat promote: pindah _private/<a>/<n>/ → <n>/).
var regDir = map[string]string{}

// PrivateInfo — info tool privat (buat tool_promote bikin proposal + verifikasi owner). ok=false kalau
// bukan tool privat / ga ada.
func PrivateInfo(name string) (owner, dir string, ok bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	s := regSpecs[name]
	if s.Scope != "private" {
		return "", "", false
	}
	return s.AgentID, regDir[name], true
}

// Promote — pindah tool privat ke shared + re-register (dipanggil promoteToolApply abis Dewan approve).
// toolsDir = root tools/. name = nama tool. Balik info hasil.
func Promote(toolsDir, name string) (map[string]any, error) {
	regMu.RLock()
	s, src := regSpecs[name], regDir[name]
	regMu.RUnlock()
	if s.Scope != "private" || src == "" {
		return nil, fmt.Errorf("tool %q bukan privat / ga ketemu (mungkin udah promoted)", name)
	}
	dst := filepath.Join(toolsDir, name)
	if _, err := os.Stat(dst); err == nil {
		return nil, fmt.Errorf("dst %q udah ada (nama bentrok shared)", dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir dst: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		return nil, fmt.Errorf("pindah folder: %w", err)
	}
	// Lepas registrasi privat → register ulang sbg shared (binary ikut pindah, CWD baru = dst).
	_ = tools.Unregister(name)
	regMu.Lock()
	delete(regSpecs, name)
	delete(regDir, name)
	regMu.Unlock()
	var names, unbuilt []string
	registerOne(dst, "", "shared", &names, &unbuilt)
	return map[string]any{"promoted": name, "from": src, "to": dst, "scope": "shared"}, nil
}

// PrivateList — semua tool privat (buat auto-propose ke Dewan). Spec.AgentID = pembuat.
func PrivateList() []Spec {
	regMu.RLock()
	defer regMu.RUnlock()
	out := []Spec{}
	for _, s := range regSpecs {
		if s.Scope == "private" {
			out = append(out, s)
		}
	}
	return out
}

// ── HEALTH + GC: seleksi alam buat tool (owner 2026-06-23) ──────────────────
// Track error_count + last_used per tool → cron prune yg sering error (rusak, mis. API berubah) /
// nganggur lama (obsolete). Persist ke tools/.health.json (lintas-restart).

type toolHealth struct {
	ErrCount     int   `json:"err_count"`
	LastUsedUnix int64 `json:"last_used_unix"`
	FirstUnix    int64 `json:"first_unix"`
}

var (
	healthMu   sync.Mutex
	healthMap  = map[string]*toolHealth{}
	healthPath string
)

func nowUnix() int64 { return time.Now().Unix() }

func loadHealth(toolsDir string) {
	healthMu.Lock()
	defer healthMu.Unlock()
	healthPath = filepath.Join(toolsDir, ".health.json")
	if raw, err := os.ReadFile(healthPath); err == nil {
		_ = json.Unmarshal(raw, &healthMap)
	}
}
func saveHealth() {
	healthMu.Lock()
	defer healthMu.Unlock()
	if healthPath == "" {
		return
	}
	if b, err := json.MarshalIndent(healthMap, "", " "); err == nil {
		_ = os.WriteFile(healthPath, b, 0o644)
	}
}
func recordUse(name string) {
	healthMu.Lock()
	h := healthMap[name]
	if h == nil {
		h = &toolHealth{FirstUnix: nowUnix()}
		healthMap[name] = h
	}
	h.LastUsedUnix = nowUnix()
	healthMu.Unlock()
}
func recordErr(name string) {
	healthMu.Lock()
	h := healthMap[name]
	if h == nil {
		h = &toolHealth{FirstUnix: nowUnix()}
		healthMap[name] = h
	}
	h.ErrCount++
	healthMu.Unlock()
}

// DeleteTool — HAPUS tool (GC/manual): unregister (langsung ga bisa di-call = deletion-aware UTAMA) +
// hapus folder + health. Balik (owner, scope) buat caller bersihin cognition. NB: bawaan (shared agentID="")
// dari repo bisa di-recreate; privat/promoted yg ke-hapus = permanen.
func DeleteTool(toolsDir, name string) (owner, scope string, err error) {
	regMu.Lock()
	s, dir := regSpecs[name], regDir[name]
	if dir == "" {
		regMu.Unlock()
		return "", "", fmt.Errorf("tool %q ga ketemu", name)
	}
	owner, scope = s.AgentID, s.Scope
	delete(regSpecs, name)
	delete(regDir, name)
	regMu.Unlock()
	_ = tools.Unregister(name) // ← deletion-aware UTAMA: ilang dari registry → ga muncul di specs → agent GA bisa akses
	_ = os.RemoveAll(dir)
	healthMu.Lock()
	delete(healthMap, name)
	healthMu.Unlock()
	saveHealth()
	addTombstone(toolsDir, name) // deletion-aware: catat mati → GC sweep quarantine cognition tiap siklus
	return owner, scope, nil
}

// GCDecision — kandidat prune (buat caller eksekusi + log + bersihin cognition).
type GCDecision struct {
	Name, Owner, Scope, Reason string
}

// GCScan — tentuin tool mana yg WAJIB mati (error-tinggi / nganggur-lama). maxErr<=0 / maxIdleDays<=0 =
// matiin kriteria itu. BAWAAN repo (shared, agentID="") di-SKIP (itu dikelola repo, bukan GC). Balik
// keputusan — caller (feature) yg DeleteTool + bersihin cognition (biar punya akses store/host).
func GCScan(maxErr, maxIdleDays int) []GCDecision {
	healthMu.Lock()
	snap := map[string]toolHealth{}
	for k, v := range healthMap {
		snap[k] = *v
	}
	healthMu.Unlock()
	var out []GCDecision
	idleCut := nowUnix() - int64(maxIdleDays)*86400
	regMu.RLock()
	defer regMu.RUnlock()
	for name, s := range regSpecs {
		if s.AgentID == "" && s.Scope == "shared" {
			continue // bawaan repo — bukan urusan GC (dikelola repo/build-tools.sh)
		}
		h := snap[name]
		reason := ""
		if maxErr > 0 && h.ErrCount >= maxErr {
			reason = fmt.Sprintf("error %dx (>=%d) — kemungkinan rusak (mis. API berubah)", h.ErrCount, maxErr)
		} else if maxIdleDays > 0 && h.LastUsedUnix > 0 && h.LastUsedUnix < idleCut {
			reason = fmt.Sprintf("nganggur >%d hari — obsolete/sementara", maxIdleDays)
		}
		if reason != "" {
			out = append(out, GCDecision{Name: name, Owner: s.AgentID, Scope: s.Scope, Reason: reason})
		}
	}
	return out
}

// ── TOMBSTONE: tool yg udah MATI (deletion-aware, owner 2026-06-23) ──────────
// Daftar nama tool yg pernah dihapus → GC feature pakai buat quarantine cognition-node tiap sweep
// (nutup celah: dream re-project tool mati dari pengalaman lama → di-quarantine LAGI). Aman: cuma
// nyentuh tool yg EKSPLISIT mati (bukan diff-registry yg bisa false-positive pas boot).
var tombMu sync.Mutex

func tombPath(toolsDir string) string { return filepath.Join(toolsDir, ".tombstones.json") }

func addTombstone(toolsDir, name string) {
	tombMu.Lock()
	defer tombMu.Unlock()
	set := map[string]bool{}
	if raw, err := os.ReadFile(tombPath(toolsDir)); err == nil {
		_ = json.Unmarshal(raw, &set)
	}
	set[name] = true
	if b, err := json.MarshalIndent(set, "", " "); err == nil {
		_ = os.WriteFile(tombPath(toolsDir), b, 0o644)
	}
}

// Tombstones — nama tool yg udah mati (buat GC cognition sweep).
func Tombstones(toolsDir string) []string {
	tombMu.Lock()
	defer tombMu.Unlock()
	set := map[string]bool{}
	if raw, err := os.ReadFile(tombPath(toolsDir)); err == nil {
		_ = json.Unmarshal(raw, &set)
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	return out
}
