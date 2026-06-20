// === LOCKED FILE (soft) === Status: STABLE (owner-approved 2026-06-15). 06-15 fixes:
//   coderTemplate fallback (clone any installed agent wasm) + coderModel honors the
//   Settings Default Model (FLOWORK_LLM_MODEL) before the Opus const. Tested. DO NOT
//   MODIFY without owner approval.
//
// coder.go — CODER: AI Utama yang BEREVOLUSI lewat BIKIN AGENT BARU (roadmap 2.2).
// PANTANGAN MUTLAK: ga sentuh file INTI. Cara evolusi = generate `.fwpack` →
// gerbang (VERIFIER → owner-approve) → install lewat pipeline plug-and-play yg
// UDAH ada. Prinsip "agent bodoh, engine pinter": LLM (Opus) cuma ngisi SPEC
// kreatif (persona/directive/kategori); ENGINE (Go deterministik) yg rakit pack
// dari TEMPLATE wasm generic — SAMA kayak cara zodiak dibikin tangan.
//
//	POST /api/coder/generate {task,model?}  → design spec (Opus) → assemble → VERIFY → stage PENDING
//	GET  /api/coder/pending                 → daftar pack nunggu approve owner
//	POST /api/coder/approve?id=<cat>        → install via installPluginPack (gerbang owner)
//	POST /api/coder/reject?id=<cat>         → buang pending
//
// Gerbang DEPLOY: generate → caps-consent (di install) → smoke → VERIFIER →
// OWNER-APPROVE. Otonomi DIRAIH lewat track-record, BUKAN dikasih gratis.
// Loopback-only. Pack di-stage ke ~/.flowork/coder-pending/ (di luar AgentsDir
// → GA ke-hot-load sampe owner approve).

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/groupsapi"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

// zipPack — bikin zip in-memory dari map path→content. Deterministik (sort key).
func zipPack(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	// sort biar output deterministik (checksum stabil).
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		f, err := zw.Create(n)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// coderModelDefault — Opus buat kerja mikir berat (roadmap 2.2). Override via
// env FLOWORK_CODER_MODEL.
const coderModelDefault = "claude-opus-4-8"

func coderModel(req string) string {
	if m := strings.TrimSpace(req); m != "" {
		return m
	}
	if m := strings.TrimSpace(os.Getenv("FLOWORK_CODER_MODEL")); m != "" {
		return m
	}
	// Honor Settings → Default Model (kv llm_default_model → FLOWORK_LLM_MODEL set at
	// boot). So AI Studio / coder follow the owner's chosen default (single source of
	// truth) instead of forcing Opus. coderModelDefault is only the last-resort fallback.
	if m := strings.TrimSpace(os.Getenv("FLOWORK_LLM_MODEL")); m != "" {
		return m
	}
	return coderModelDefault
}

// AgentSpec — SPEC kreatif yang LLM isi (forced-tool). Engine rakit pack dari ini.
type AgentSpec struct {
	CategoryID      string `json:"category_id"`
	Name            string `json:"name"`
	Icon            string `json:"icon"`
	TriggerHint     string `json:"trigger_hint"`
	SynthDirective  string `json:"synth_directive"`
	WorkerDirective string `json:"worker_directive"`
	SynthPersona    string `json:"synth_persona"`
	WorkerRole      string `json:"worker_role"`
	WorkerPersona   string `json:"worker_persona"`
}

var coderCatRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}$`)

// validate — tolak spec ngaco SEBELUM assemble.
func (s *AgentSpec) validate() string {
	if !coderCatRe.MatchString(s.CategoryID) {
		return "category_id invalid (^[a-z0-9][a-z0-9-]{1,30}$): " + s.CategoryID
	}
	for k, v := range map[string]string{
		"name": s.Name, "trigger_hint": s.TriggerHint, "synth_directive": s.SynthDirective,
		"synth_persona": s.SynthPersona, "worker_role": s.WorkerRole, "worker_persona": s.WorkerPersona,
	} {
		if strings.TrimSpace(v) == "" {
			return "field kosong: " + k
		}
	}
	return ""
}

// coderPendingDir — ~/.flowork/coder-pending/ (DI LUAR AgentsDir → ga ke-hot-load
// sampe owner approve). Mirror pola dropbox.
func coderPendingDir() string {
	return filepath.Join(filepath.Dir(loader.AgentsDir()), "coder-pending")
}

// coderDesignSpec — panggil router (Opus) dengan tool_choice DIPAKSA keluarin
// AgentSpec terstruktur. Pola sama classifier mr-flow (anti free-text halu).
func coderDesignSpec(ctx context.Context, task, model string) (AgentSpec, error) {
	var spec AgentSpec
	designTool := map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "design_app",
			"description": "Rancang 1 app task Flowork (crew: 1 worker + 1 synthesizer) dari deskripsi user. WAJIB dipanggil sekali.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category_id":      map[string]any{"type": "string", "description": "id slug unik, lowercase-dash (mis. 'pantun', 'resep-masak'). 2-31 char."},
					"name":             map[string]any{"type": "string", "description": "nama app human-readable (mis. 'Generator Pantun')"},
					"icon":             map[string]any{"type": "string", "description": "1 emoji yang cocok"},
					"trigger_hint":     map[string]any{"type": "string", "description": "kapan app ini dipanggil (buat classifier route), beri contoh."},
					"synth_directive":  map[string]any{"type": "string", "description": "instruksi FORMAT output final synthesizer (struktur, gaya). SINGKAT."},
					"worker_directive": map[string]any{"type": "string", "description": "instruksi cara worker kerja. Kalau KREATIF (ga butuh data real), bilang 'ngarang itu tugasnya'. Kalau ANALISIS, suruh cari data real."},
					"synth_persona":    map[string]any{"type": "string", "description": "persona/system-prompt synthesizer (perakit output final)."},
					"worker_role":      map[string]any{"type": "string", "description": "label peran worker (mis. 'penyair', 'periset')."},
					"worker_persona":   map[string]any{"type": "string", "description": "persona/system-prompt worker."},
				},
				"required": []string{"category_id", "name", "icon", "trigger_hint", "synth_directive", "worker_directive", "synth_persona", "worker_role", "worker_persona"},
			},
		},
	}
	args, err := routerForcedTool(ctx, model,
		"Lo arsitek app Flowork. Rancang app task (crew 1 worker + 1 synth) dari permintaan user. Persona & directive HARUS sesuai domain app. Bahasa Indonesia. RINGKAS (anti over-prompt).",
		"Bikin app buat: "+task, designTool, "design_app", 1500)
	if err != nil {
		return spec, err
	}
	if err := json.Unmarshal(args, &spec); err != nil {
		return spec, fmt.Errorf("decode spec args: %w", err)
	}
	spec.CategoryID = strings.ToLower(strings.TrimSpace(spec.CategoryID))
	return spec, nil
}

func trimStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// coderTemplate — cari wasm + manifest TEMPLATE generic (worker & synth) dari
// agent built-in yang udah ada di AgentsDir. Generic crew main.go IDENTIK lintas
// agent → wasm apapun cocok jadi template. Balik (wasm, manifestRaw, err).
func coderTemplate(role string) ([]byte, []byte, error) {
	// TEMPLATE BARU KANONIK (owner 2026-06-20): templates/agent-template = cetakan
	// fresh sesuai standar (DB-config, no .md). Dipakai DULUAN biar agent baru lahir
	// dari template current, ga gantung ke agen built-in yg mungkin udah dihapus.
	// Manifest skeleton (id="TEMPLATE_AGENT_ID") di-swap caller (swapManifest).
	if wasm, e1 := os.ReadFile(filepath.Join("templates", "agent-template", "agent.wasm")); e1 == nil && len(wasm) > 0 {
		if man, e2 := os.ReadFile(filepath.Join("templates", "agent-template", "manifest.json")); e2 == nil {
			return wasm, man, nil
		}
	}
	// kandidat stabil built-in (worker pertama, synth ke-2). Fallback: scan apa aja.
	cands := map[string][]string{
		"worker": {"saham-fundamental", "crypto-fundamental", "music-riset"},
		"synth":  {"saham-sinteser", "crypto-sinteser", "music-sinteser"},
	}[role]
	root := loader.AgentsDir()
	for _, id := range cands {
		dir := filepath.Join(root, id+".fwagent")
		wasm, e1 := os.ReadFile(filepath.Join(dir, "agent.wasm"))
		man, e2 := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if e1 == nil && e2 == nil && len(wasm) > 0 {
			return wasm, man, nil
		}
	}
	// Fallback (the comment's intent "scan apa aja"): clone ANY installed agent's
	// wasm as the generic template — behavior comes from plugin.json persona/
	// directive, not the wasm ("agent bodoh, engine pinter"). Skip channels /
	// orchestrators / self so we template off a plain worker-style agent.
	skip := map[string]bool{
		"telegram-channel": true, "discord-channel": true, "slack-channel": true,
		"whatsapp-channel": true, "operator-shutdown": true, "operator-komputer": true,
		"mr-flow": true, "mr-flow-next": true, "flowork-architect": true,
	}
	if ents, e := os.ReadDir(root); e == nil {
		for _, ent := range ents {
			if !ent.IsDir() || !strings.HasSuffix(ent.Name(), ".fwagent") {
				continue
			}
			if skip[strings.TrimSuffix(ent.Name(), ".fwagent")] {
				continue
			}
			dir := filepath.Join(root, ent.Name())
			wasm, e1 := os.ReadFile(filepath.Join(dir, "agent.wasm"))
			man, e2 := os.ReadFile(filepath.Join(dir, "manifest.json"))
			if e1 == nil && e2 == nil && len(wasm) > 0 {
				return wasm, man, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("template %s ga ketemu (built-in agent belum ke-build?)", role)
}

// coderAssemblePack — rakit .fwpack dari spec + template wasm. DETERMINISTIK.
// Pack: plugin.json (kategori + crew + persona + directive) + agents/<id>/
// {agent.wasm, manifest.json}. Persona ikut via plugin.json (fix P0).
func coderAssemblePack(spec AgentSpec) ([]byte, error) {
	workerWasm, workerMan, err := coderTemplate("worker")
	if err != nil {
		return nil, err
	}
	synthWasm, synthMan, err := coderTemplate("synth")
	if err != nil {
		return nil, err
	}
	workerID := spec.CategoryID + "-worker"
	synthID := spec.CategoryID + "-synth"

	// manifest agent: copy template, swap id + display_name (caps proven dari template).
	mkManifest := func(tmpl []byte, id, display string) ([]byte, error) {
		m := map[string]any{} // non-nil: Unmarshal("null") = no-op → tanpa init, write bawah panic
		if e := json.Unmarshal(tmpl, &m); e != nil {
			return nil, e
		}
		m["id"] = id
		m["display_name"] = display
		return json.MarshalIndent(m, "", "  ")
	}
	workerManifest, err := mkManifest(workerMan, workerID, spec.Name+" — "+spec.WorkerRole)
	if err != nil {
		return nil, fmt.Errorf("worker manifest: %w", err)
	}
	synthManifest, err := mkManifest(synthMan, synthID, spec.Name+" — synthesizer")
	if err != nil {
		return nil, fmt.Errorf("synth manifest: %w", err)
	}

	// plugin.json
	man := pluginManifest{ID: spec.CategoryID + "-pack", Name: spec.Name, Version: "1.0.0", Author: "flowork-coder"}
	man.Category.ID = spec.CategoryID
	man.Category.Name = spec.Name
	man.Category.Icon = spec.Icon
	man.Category.TriggerHint = spec.TriggerHint
	man.Category.SynthDirective = spec.SynthDirective
	man.Category.WorkerDirective = spec.WorkerDirective
	man.Crew = []pluginCrewMember{
		{AgentID: synthID, RoleLabel: "synthesizer", Kind: "synth", Persona: spec.SynthPersona},
		{AgentID: workerID, RoleLabel: spec.WorkerRole, Kind: "worker", Persona: spec.WorkerPersona},
	}
	pluginJSON, _ := json.MarshalIndent(man, "", "  ")

	// zip
	return zipPack(map[string][]byte{
		"plugin.json":                           pluginJSON,
		"agents/" + workerID + "/agent.wasm":    workerWasm,
		"agents/" + workerID + "/manifest.json": workerManifest,
		"agents/" + synthID + "/agent.wasm":     synthWasm,
		"agents/" + synthID + "/manifest.json":  synthManifest,
	})
}

// coderGenerate — pipeline penuh: design (Opus) → assemble → VERIFY → stage pending.
func coderGenerate(ctx context.Context, task, model string) (map[string]any, error) {
	spec, err := coderDesignSpec(ctx, task, model)
	if err != nil {
		return nil, err
	}
	if msg := spec.validate(); msg != "" {
		return nil, fmt.Errorf("spec invalid: %s", msg)
	}
	pack, err := coderAssemblePack(spec)
	if err != nil {
		return nil, err
	}
	verdict := verifyPackStatic(pack)
	// LLM-judge adversarial (semantik) atas pack yang BARU dirakit — "desain BENER?".
	// Gagal judge = ga fatal (static verdict tetep jalan).
	judge, jerr := verifierJudge(ctx, model, packAppDesc(pack))

	// stage ke pending (di luar AgentsDir). pack + meta.
	dir := coderPendingDir()
	if e := os.MkdirAll(dir, 0o755); e != nil {
		return nil, fmt.Errorf("mkdir pending: %w", e)
	}
	packPath := filepath.Join(dir, spec.CategoryID+".fwpack")
	if e := os.WriteFile(packPath, pack, 0o644); e != nil {
		return nil, fmt.Errorf("write pack: %w", e)
	}
	meta := map[string]any{"id": spec.CategoryID, "task": task, "model": model, "spec": spec, "verify": verdict}
	out := map[string]any{
		"ok": true, "pending_id": spec.CategoryID, "spec": spec, "verify": verdict,
		"next": "owner review di Approval Queue → approve (install) / reject.",
	}
	if jerr == nil {
		meta["judge"] = judge
		out["judge"] = judge
	} else {
		meta["judge_error"] = jerr.Error()
	}
	metaRaw, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, spec.CategoryID+".json"), metaRaw, 0o644)
	return out, nil
}

// ── HTTP handlers ────────────────────────────────────────────────────────────

func coderGenerateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Task  string `json:"task"`
			Model string `json:"model"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		if strings.TrimSpace(body.Task) == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "task required"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 200*time.Second)
		defer cancel()
		res, err := coderGenerate(ctx, body.Task, coderModel(body.Model))
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, res)
	}
}

// coderPendingHandler — GET daftar pending (baca meta json di coder-pending/).
func coderPendingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir := coderPendingDir()
		entries, _ := os.ReadDir(dir)
		out := []map[string]any{}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var meta map[string]any
			if json.Unmarshal(raw, &meta) == nil {
				out = append(out, meta)
			}
		}
		tfWriteJSON(w, 0, map[string]any{"pending": out})
	}
}

// coderApproveHandler — POST ?id=<cat>. Owner approve → install via pipeline yg
// UDAH ada (installPluginPack), TRANSAKSIONAL. Sukses → buang pending.
func coderApproveHandler(host *kernelhost.Host, store *floworkdb.Store, groups *groupsapi.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !coderCatRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		packPath := filepath.Join(coderPendingDir(), id+".fwpack")
		raw, err := os.ReadFile(packPath)
		if err != nil {
			tfWriteJSON(w, http.StatusNotFound, map[string]any{"error": "pending ga ada: " + id})
			return
		}
		// ENFORCE the verifier verdict — the VERIFIER is a real gate, not just a label.
		// A 'blocked' pack must NOT install on a plain approve; the owner may consciously
		// force it with ?override=1 (a deliberate, logged choice).
		if v := verifyPackStatic(raw); v.Status == "blocked" && r.URL.Query().Get("override") != "1" {
			tfWriteJSON(w, http.StatusForbidden, map[string]any{
				"ok":      false,
				"blocked": true,
				"error":   "VERIFIER blocked this pack — review the findings, then re-approve with override to force-install",
				"verify":  v,
			})
			return
		}
		if r.URL.Query().Get("override") == "1" {
			fmt.Fprintf(os.Stderr, "[coder] owner OVERRIDE: installing pack %q despite a blocked verdict\n", id)
		}
		// owner approve = trusted → approve caps (gerbang manusia udah lewat).
		res := installPluginPack(host, store, raw, true)
		if res.status != 0 {
			tfWriteJSON(w, res.status, res.body)
			return
		}
		// P3 (owner rule): agent WAJIB jadi GROUP — alur mr-flow → group → agent → lapor TG.
		// Coder dulu cuma bikin category-crew (lepas dari orchestrator group). Sekarang
		// auto-promote ke group (CreateGroup set kv group=1 + roster + sync). Best-effort:
		// gagal-group → log, install tetap sukses (gak rollback agent yg udah ke-install).
		if groups != nil {
			catID, _ := res.body["category"].(string)
			catName, _ := res.body["cat_name"].(string)
			synth, _ := res.body["synth"].(string)
			members, _ := res.body["crew_workers"].([]string)
			if catID != "" {
				if gerr := groups.CreateGroup(catID, catName, members, synth, ""); gerr != nil {
					fmt.Fprintf(os.Stderr, "[coder] WARN auto-group %q gagal: %v (install tetap sukses)\n", catID, gerr)
					res.body["grouped"] = false
				} else {
					groups.SyncToOrchestrator()
					res.body["grouped"] = true
				}
			}
		}
		// sukses → buang pending (pack + meta).
		_ = os.Remove(packPath)
		_ = os.Remove(filepath.Join(coderPendingDir(), id+".json"))
		res.body["approved"] = id
		tfWriteJSON(w, 0, res.body)
	}
}

// coderRejectHandler — POST ?id=<cat>. Buang pending (discard bersih).
func coderRejectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if !coderCatRe.MatchString(id) {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "id invalid"})
			return
		}
		_ = os.Remove(filepath.Join(coderPendingDir(), id+".fwpack"))
		_ = os.Remove(filepath.Join(coderPendingDir(), id+".json"))
		tfWriteJSON(w, 0, map[string]any{"ok": true, "rejected": id})
	}
}
