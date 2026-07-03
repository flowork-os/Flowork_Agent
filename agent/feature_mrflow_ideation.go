// feature_mrflow_ideation.go — SARAF IDE fase-2 (owner 2026-07-03): mr-flow PROAKTIF ngusul ide
// evolusi FORWARD-LOOKING pas idle. Beda dari reflect-proposer (yg nambang mistakes = BACKWARD,
// "benerin yg udah rusak"): ini mr-flow (penyambung ide/kebutuhan owner) mikir "kapabilitas apa
// yang bakal ngebantu owner KE DEPAN" dari konteks percakapan + friksi.
//
// SUPER GATED (owner minta hati-hati):
//   - DEFAULT OFF — nyala cuma kalau FLOWORK_MRFLOW_IDEATION=1/on.
//   - RATE-LIMIT — 1 putaran / FLOWORK_MRFLOW_IDEATION_MIN menit (default 360 = 6 jam).
//   - 1 IDE per putaran (anti-spam), + skip kalau backlog 'proposed' udah penuh.
//   - Lewat PIPELINE BERGERBANG normal (pilar → Dewan → review owner) — kalaupun idenya jelek,
//     gate nyaring + owner mutusin. mr-flow cuma NGUSUL, ga nerapin.
//   - Additive sibling (RegisterFeature seam), NOL sentuh file beku. Reversible (hapus file = ilang).
// 📄 Dok: FLowork_os/lock/evolusi-grounded.md
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
)

const (
	mrflowIdeationAgentID    = "mr-flow"
	mrflowIdeationKVLast     = "mrflow_ideation_last"
	mrflowIdeationDefaultMin = 360 // 6 jam
	mrflowIdeationBacklogCap = 12  // 'proposed' >= ini → skip (jangan numpuk)
)

func mrflowIdeationEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_MRFLOW_IDEATION"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

func mrflowIdeationMin() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FLOWORK_MRFLOW_IDEATION_MIN"))); err == nil && n >= 30 {
		return n
	}
	return mrflowIdeationDefaultMin
}

func init() {
	RegisterFeature(Feature{Name: "mrflow-ideation", Phase: PhaseRoute, Apply: func(d *Deps) {
		if d.Host == nil || d.FDB == nil {
			return
		}
		fdb := d.FDB
		ctx := d.Ctx
		// Endpoint force-run (owner tes manual / trigger sadar). Tetep hormatin switch OFF-default.
		d.Mux.HandleFunc("/api/evolve/ideation-run", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runMrflowIdeation(ctx, fdb, true))
		})
		// Goroutine SELALU jalan; cek switch di dalem loop → owner bisa nyalain tanpa restart.
		go mrflowIdeationLoop(ctx, fdb)
	}})
}

func mrflowIdeationLoop(ctx context.Context, fdb *floworkdb.Store) {
	t := time.NewTicker(15 * time.Minute) // tick kasar; interval asli dari env + rate-limit KV
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if out := runMrflowIdeation(ctx, fdb, false); out["ok"] == true {
				log.Printf("[mrflow-ideation] %v", out)
			}
		}
	}
}

type mrflowIdea struct {
	Title      string `json:"title"`
	Rationale  string `json:"rationale"`
	Kind       string `json:"kind"`
	TargetFile string `json:"target_file"`
	Risk       string `json:"risk"`
}

// runMrflowIdeation — sekali putaran. force=true (buat tes) lewatin interval TAPI TETEP hormatin switch.
func runMrflowIdeation(ctx context.Context, fdb *floworkdb.Store, force bool) map[string]any {
	if !mrflowIdeationEnabled() {
		return map[string]any{"skipped": "switch OFF (FLOWORK_MRFLOW_IDEATION)"}
	}
	if !force {
		if last, _ := fdb.GetKV(mrflowIdeationKVLast); strings.TrimSpace(last) != "" {
			if ts, e := time.Parse(time.RFC3339, strings.TrimSpace(last)); e == nil {
				if time.Since(ts) < time.Duration(mrflowIdeationMin())*time.Minute {
					return map[string]any{"skipped": "belum waktunya"}
				}
			}
		}
	}
	dir := filepath.Join(loader.AgentsDir(), mrflowIdeationAgentID+".fwagent")
	store, err := agentdb.Open(agentdb.Resolve(mrflowIdeationAgentID, dir))
	if err != nil {
		return map[string]any{"error": "store: " + err.Error()}
	}
	defer store.Close()
	// Stamp DI AWAL (anti-spam walau gagal di tengah).
	_ = fdb.SetKV(mrflowIdeationKVLast, time.Now().UTC().Format(time.RFC3339))

	if n, _ := store.CountProposalsByStatus("proposed"); n >= mrflowIdeationBacklogCap {
		return map[string]any{"skipped": "backlog 'proposed' penuh"}
	}
	groundCtx := buildMrflowIdeationContext(store)
	if strings.TrimSpace(groundCtx) == "" {
		return map[string]any{"skipped": "nol sinyal grounding (percakapan+friksi kosong)"}
	}
	idea, err := mrflowGenIdea(ctx, groundCtx)
	if err != nil {
		return map[string]any{"error": "gen: " + err.Error()}
	}
	if idea == nil || strings.TrimSpace(idea.Rationale) == "" || strings.TrimSpace(idea.Title) == "" {
		return map[string]any{"skipped": "mr-flow ga nemu ide layak"}
	}
	kind := strings.ToLower(strings.TrimSpace(idea.Kind))
	isCore := kind == "fix" || kind == "refactor" || kind == "doc" || kind == "test"
	if kind == "" {
		kind = "add-skill"
	}
	target := strings.TrimSpace(idea.TargetFile)
	if target == "" && !isCore {
		target = "NEW:" + ideationSlug(idea.Title)
	}
	if target == "" {
		return map[string]any{"skipped": "core idea tanpa target_file"}
	}
	// DEDUP: target udah aktif → jangan bikin lagi.
	for _, ex := range activeIdeationTargets(store) {
		if strings.EqualFold(ex, target) {
			return map[string]any{"skipped": "target udah ada di backlog"}
		}
	}
	risk := strings.ToLower(strings.TrimSpace(idea.Risk))
	if risk != "low" && risk != "high" {
		risk = "medium"
	}
	var rb [8]byte
	_, _ = rand.Read(rb[:])
	p := agentdb.EvolveProposal{
		ID:         "ev_mrflow_" + hex.EncodeToString(rb[:]),
		Goal:       "[IDE mr-flow — proaktif] " + idea.Title,
		TargetFile: target, Kind: kind,
		Rationale: "[IDE mr-flow — forward-looking, grounded ke percakapan owner] " + idea.Rationale,
		Risk:      risk, Status: "proposed",
		Pillar:    strings.Join(agentdb.ClassifyPillars(idea.Title+" "+kind+" "+idea.Rationale+" "+target), ","),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := store.AddEvolveProposal(p); err != nil {
		return map[string]any{"error": "simpan: " + err.Error()}
	}
	return map[string]any{"ok": true, "id": p.ID, "target": target, "kind": kind, "pillar": p.Pillar}
}

// buildMrflowIdeationContext — sinyal FORWARD-looking: percakapan owner terbaru (apa yg lagi diurusin)
// + friksi (mistakes). Percakapan = intent (utama); mistakes = pelengkap. Truncate biar hemat token.
func buildMrflowIdeationContext(store *agentdb.Store) string {
	var b strings.Builder
	if its, _ := store.ListInteractions("", "", 12); len(its) > 0 {
		b.WriteString("## PERCAKAPAN OWNER TERBARU (apa yang lagi dia urusin/butuhin):\n")
		n := 0
		for _, it := range its {
			c := strings.TrimSpace(it.Content)
			if c == "" {
				continue
			}
			if len(c) > 180 {
				c = c[:180] + "…"
			}
			b.WriteString("- (" + it.Direction + ") " + c + "\n")
			if n++; n >= 12 {
				break
			}
		}
	}
	if ms, _ := store.ListMistakesEligibleForPromote(1, 5); len(ms) > 0 {
		b.WriteString("\n## FRIKSI TEREKAM (pelengkap):\n")
		for _, m := range ms {
			line := "- [" + m.Category + "] " + m.Title
			if len(line) > 120 {
				line = line[:120] + "…"
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// mrflowGenIdea — LLM (model proposer) diframing sebagai mr-flow → SATU ide forward-looking. JSON tunggal.
func mrflowGenIdea(ctx context.Context, groundCtx string) (*mrflowIdea, error) {
	cctx, cancel := context.WithTimeout(ctx, 200*time.Second)
	defer cancel()
	sys := "Kamu mr-flow — penyambung ide & kebutuhan owner Flowork. Dari konteks (percakapan owner + " +
		"friksi), usulkan TEPAT SATU ide evolusi FORWARD-LOOKING: kapabilitas/skill yang bakal ngebantu " +
		"owner KE DEPAN (bukan sekadar benerin error lama). Harus KONKRET, ADDITIF, aman. " +
		`Balas HANYA 1 objek JSON: {"title":"nama singkat ide","rationale":"1-2 kalimat: apa + kenapa ngebantu owner","kind":"add-skill|add-agent|add-app","target_file":"kosongin buat behavior (auto), atau NEW:<nama>","risk":"low|medium|high"}. ` +
		"PREFER kind=add-skill. Kalau ga ada ide yang beneran ngebantu & grounded, balas {}. Ga usah prosa, JSON aja."
	res, err := routerChatSafe(cctx, evoCoderModel(), []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": groundCtx},
	}, nil, 500)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(res.Content)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var idea mrflowIdea
	if err := json.Unmarshal([]byte(raw), &idea); err != nil {
		return nil, nil // parse gagal / {} → ga ada ide (bukan error keras)
	}
	return &idea, nil
}

func activeIdeationTargets(store *agentdb.Store) []string {
	out := []string{}
	rows, err := store.ListEvolveProposals(200)
	if err != nil {
		return out
	}
	for _, r := range rows {
		if s, _ := r["status"].(string); s == "rejected" {
			continue
		}
		if tf, _ := r["target_file"].(string); strings.TrimSpace(tf) != "" {
			out = append(out, tf)
		}
	}
	return out
}

// ideationSlug — judul → nama-skill aman (huruf-kecil, spasi→'-'). Lokal (slug builtins beda package).
func ideationSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-' || r == '_' || r == '/':
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "ide-mrflow"
	}
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	return out
}
