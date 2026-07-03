// === FROZEN === Status: STABLE — bridge ide owner→evolusi. Owner-approved 2026-06-20; dibikin
// FORGIVING 2026-07-03 (terima title/description + auto NEW:<slug>) → di-freeze setelah unit test PASS.
// 📄 Dok: FLowork_os/lock/evolusi-grounded.md
package builtins

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

// evolve_propose_tool.go — owner 2026-06-20: "mr-flow kasih evolve_proposals; gw aktif diskusi
// sama mr-flow, dia salurkan IDE GW ke team evolute biar di-review". Tool ini bikin mr-flow bisa
// nyetor ide owner langsung jadi proposal evolusi (status=proposed) → masuk pipeline normal:
// DEWAN review → core-apply (loop coder↔reviewer) → STAGE. Ditandai [IDE OWNER] biar prioritas +
// ke-skip auto-reject classifier (classifier cuma jalan di jalur proposer/reflect, BUKAN di sini).

func init() { tools.Register(&evolveProposeTool{}) }

type evolveProposeTool struct{}

func (evolveProposeTool) Name() string       { return "evolve_propose" }
func (evolveProposeTool) Capability() string { return "state:write" }
func (evolveProposeTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Salurkan IDE OWNER jadi proposal evolusi (masuk backlog → Dewan review → core-apply). Pakai pas owner kasih ide perbaikan/fitur lewat chat & minta diteruskan ke team evolusi. Ditandai [IDE OWNER] (prioritas, ga kena auto-reject classifier). Minimal isi 'title' + 'rationale' — buat ide skill/behavior, target_file otomatis dibikin dari judul.",
		Params: []tools.Param{
			{Name: "title", Type: tools.ParamString, Description: "judul singkat ide (mis. 'Cek koneksi router sebelum LLM'). Buat add-skill jadi nama skill-nya."},
			{Name: "rationale", Type: tools.ParamString, Description: "kenapa ide ini penting (ringkas, 1-2 kalimat). Alias: description/idea.", Required: true},
			{Name: "kind", Type: tools.ParamString, Description: "jenis: add-skill|add-agent|add-app (behavior, DEFAULT add-skill) atau fix|refactor|doc|test (core — WAJIB kasih target_file berupa path repo asli)"},
			{Name: "target_file", Type: tools.ParamString, Description: "OPSIONAL buat behavior (otomatis 'NEW:<judul>'). WAJIB buat core: path repo asli (mis. internal/agentdb/foo.go)."},
			{Name: "goal", Type: tools.ParamString, Description: "konteks/tujuan ide (optional)"},
			{Name: "risk", Type: tools.ParamString, Description: "low|medium|high (default medium)"},
		},
		Returns: "{ok, id, status, pillar}",
	}
}

func (evolveProposeTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}
	target := strings.TrimSpace(asString(args["target_file"]))
	// FORGIVING (cabut akar 2026-07-03): LLM sering manggil pakai title/description, bukan
	// target_file/rationale. Terima alias + auto-turunin target buat ide behavior biar tool
	// nyambung sama cara natural mr-flow manggil (dulu: gagal validasi → ide owner ilang).
	title := strings.TrimSpace(asString(args["title"]))
	if title == "" {
		title = strings.TrimSpace(asString(args["name"]))
	}
	rationale := strings.TrimSpace(asString(args["rationale"]))
	for _, k := range []string{"description", "idea", "detail", "desc"} {
		if rationale == "" {
			rationale = strings.TrimSpace(asString(args[k]))
		}
	}
	kind := strings.ToLower(strings.TrimSpace(asString(args["kind"])))
	isCore := kind == "fix" || kind == "refactor" || kind == "doc" || kind == "test"
	if kind == "" {
		kind = "add-skill" // default = behavior (langsung apply di edisi ini, aman)
	}
	// rationale/title saling-isi biar ide minimal tetep lolos
	if rationale == "" {
		rationale = title
	}
	if title == "" {
		title = firstWords(rationale, 7)
	}
	// target: behavior → auto 'NEW:<slug judul>'. core → wajib path asli.
	if target == "" && !isCore {
		target = "NEW:" + slugify(title)
	}
	if target == "" || rationale == "" {
		return tools.Result{}, fmt.Errorf("kasih minimal 'title'+'rationale' (buat core: 'target_file' path repo asli juga)")
	}
	risk := strings.ToLower(strings.TrimSpace(asString(args["risk"])))
	if risk != "low" && risk != "high" {
		risk = "medium"
	}
	goal := strings.TrimSpace(asString(args["goal"]))
	if goal == "" {
		goal = "ide owner via mr-flow"
	}

	var b [8]byte
	_, _ = rand.Read(b[:])
	id := "ev_owner_" + hex.EncodeToString(b[:])
	// Pilar otomatis biar ke-tag (bukan "ngelantur") — ide owner tetep relevan ke 5 pilar.
	pillars := agentdb.ClassifyPillars(goal + " " + kind + " " + rationale + " " + target)
	p := agentdb.EvolveProposal{
		ID: id, Goal: "[IDE OWNER] " + goal, TargetFile: target, Kind: kind,
		Rationale: "[IDE OWNER — prioritas, sudah di-vouch owner] " + rationale,
		Risk:      risk, Status: "proposed", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Pillar: strings.Join(pillars, ","),
	}
	if err := store.AddEvolveProposal(p); err != nil {
		return tools.Result{}, fmt.Errorf("simpan proposal: %w", err)
	}
	return tools.Result{Output: map[string]any{
		"ok": true, "id": id, "status": "proposed", "pillar": p.Pillar, "target_file": target, "kind": kind,
		"note": "Ide owner masuk backlog evolusi → bakal di-review Dewan + (kalau approved) di-code+review team. Cek tab Evolution.",
	}}, nil
}

// slugify — judul → nama-skill aman (huruf-kecil, spasi→'-', buang non-alnum). Buat NEW:<slug>.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "ide-evolusi"
	}
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	return out
}

// firstWords — N kata pertama dari s (fallback judul dari rationale).
func firstWords(s string, n int) string {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}
