// selfevolve.go — R7 SELF-EVOLUTION fase-1 (refleksi-diri → backlog usulan). Plug-in.
// Owner-approved 2026-06-15 (FASE 2 autonomi). Organisme BACA self-map semantik (R6) →
// architect/LLM USULIN perbaikan konkret → simpan ke evolve_proposal. FASE-1 = USULAN
// doang (NOL ubah kode/sistem) → aman, sekaligus NGUMPULIN KARMA (kualitas usulan).
// Eksekusi (sandbox→test-gate→auto-commit) = fase-2, di-GATE karma + scope non-locked.
// LLM di-INJECT dari main (decoupling, sama kayak codemap_semantic R6).

package agentmgr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/httpx"
)

// EvolveKarmaThreshold — minimum siklus refleksi sukses sebelum mode AUTO boleh commit.
// Owner-default 2026-06-15: ≥20 sukses + rasio ≥90%. Gate berlapis: GUI toggle + karma + model.
const EvolveKarmaThreshold = 20

// EvolveGateDeps — di-inject dari main: KV (mode toggle) + cek model kuat (anti-lokal)
// + edisi (dev = evolusi penuh termasuk core; public = behavior-layer aja, core auto-update).
type EvolveGateDeps struct {
	KVGet       func(string) (string, error)
	KVSet       func(string, string) error
	ModelStrong func() (bool, string) // (cloud kuat?, catatan) — guard anti LLM-lokal
	Edition     func() string         // "dev" | "public" (default public = aman)
}

func evolveEdition(dep EvolveGateDeps) string {
	if dep.Edition == nil {
		return "public"
	}
	if e := strings.ToLower(strings.TrimSpace(dep.Edition())); e == "dev" {
		return "dev"
	}
	return "public"
}

// EvolveCoreChangeAllowed — boleh AUTO-commit perubahan CORE (Go/JS in-repo)?
// HANYA edisi dev + semua gate auto lolos. Public: SELALU false (core = auto-update upstream).
func EvolveCoreChangeAllowed(dep EvolveGateDeps) (bool, string) {
	if evolveEdition(dep) != "dev" {
		return false, "edisi public — core via auto-update upstream, tidak self-edit (anti divergen git)"
	}
	return EvolveAutoCommitAllowed(dep)
}

func evolveMode(dep EvolveGateDeps) string {
	if dep.KVGet == nil {
		return "off"
	}
	m, _ := dep.KVGet("evolve_mode")
	if m = strings.ToLower(strings.TrimSpace(m)); m == "stage" || m == "auto" {
		return m
	}
	return "off"
}

// evolveKarmaReady — track-record refleksi cukup matang buat AUTO? (≥threshold + rasio ≥90%).
func evolveKarmaReady() (ready bool, okV, failV float64) {
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		return false, 0, 0
	}
	defer store.Close()
	ok, _ := store.GetKarma("evolve_reflect_ok")
	fail, _ := store.GetKarma("evolve_reflect_fail")
	okV, failV = ok.MetricValue, fail.MetricValue
	ratio := 1.0
	if okV+failV > 0 {
		ratio = okV / (okV + failV)
	}
	return okV >= EvolveKarmaThreshold && ratio >= 0.9, okV, failV
}

// EvolveAutoCommitAllowed — GATE BERLAPIS dipakai engine eksekusi (fase-2b) SEBELUM commit.
// Semua wajib: mode=AUTO + karma matang + model cloud kuat (BUKAN lokal). Gagal satu → false.
func EvolveAutoCommitAllowed(dep EvolveGateDeps) (bool, string) {
	if evolveMode(dep) != "auto" {
		return false, "mode bukan AUTO (owner belum arm)"
	}
	ready, okV, _ := evolveKarmaReady()
	if !ready {
		return false, fmt.Sprintf("karma belum matang (%.0f/%d sukses)", okV, EvolveKarmaThreshold)
	}
	if dep.ModelStrong != nil {
		if strong, note := dep.ModelStrong(); !strong {
			return false, "model lemah/lokal — auto-commit diblok: " + note
		}
	}
	return true, "ok"
}

// EvolveBehaviorApplyAllowed — GATE buat APPLY proposal BEHAVIOR-LAYER (add-agent/skill/app).
// Lapisan ~/.flowork DI LUAR git → additive + reversible (tinggal hapus agent/app) → lebih
// longgar dari core-commit:
//   - manual (owner klik tombol Apply): cukup mode != off (armed) + model kuat (anti-lokal —
//     jangan biarin Gemma rusak bikin agent sampah). Owner in-the-loop → karma TIDAK diwajibin.
//   - auto (cron/terjadwal): full gate EvolveAutoCommitAllowed (mode=auto + karma + model).
// Beda dari core-apply (EvolveCoreChangeAllowed) yg 🔴 di-git + DEV-only.
func EvolveBehaviorApplyAllowed(dep EvolveGateDeps, auto bool) (bool, string) {
	if auto {
		return EvolveAutoCommitAllowed(dep)
	}
	if evolveMode(dep) == "off" {
		return false, "mode OFF — arm dulu (stage/auto) di tab Evolution sebelum apply"
	}
	if dep.ModelStrong != nil {
		if strong, note := dep.ModelStrong(); !strong {
			return false, "model lemah/lokal — apply diblok biar gak bikin agent sampah: " + note
		}
	}
	return true, "ok"
}

// EvolveApplier — di-INJECT dari main (decoupling, sama pola EvolveProposer/summarizer).
// Dikasih proposal yg lolos gate → BANGUN artefak behavior-layer (reuse architect:
// add-agent→team, add-app→HTML app, add-skill→SKILL.md) ke ~/.flowork. Balikin ringkasan
// hasil. agentmgr SENGAJA ga tau soal architect (isolasi) — main yg nyuntik kemampuannya.
type EvolveApplier func(ctx context.Context, p agentdb.EvolveProposal) (map[string]any, error)

// EvolveApplyHandler — POST /api/evolve/apply?id=<proposalID>[&auto=1]. Engine eksekusi
// fase-2b BEHAVIOR-LAYER (AMAN, additive, di luar git). Idempoten: status 'applied' → no-op.
func EvolveApplyHandler(dep EvolveGateDeps, apply EvolveApplier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
			return
		}
		if apply == nil {
			httpx.WriteJSON(w, map[string]any{"error": "applier not wired"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			httpx.WriteJSON(w, map[string]any{"error": "param id wajib"})
			return
		}
		auto := r.URL.Query().Get("auto") == "1"
		// GATE dulu (saklar + model), SEBELUM buka store / panggil LLM.
		if ok, why := EvolveBehaviorApplyAllowed(dep, auto); !ok {
			httpx.WriteJSON(w, map[string]any{"error": "gate behavior-apply nolak: " + why})
			return
		}
		store, err := openAgentStore(defaultAgentID)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		defer store.Close()
		p, found, gerr := store.GetEvolveProposal(id)
		if gerr != nil {
			httpx.WriteJSON(w, map[string]any{"error": gerr.Error()})
			return
		}
		if !found {
			httpx.WriteJSON(w, map[string]any{"error": "proposal " + id + " ga ketemu"})
			return
		}
		if p.Status == "applied" {
			httpx.WriteJSON(w, map[string]any{"ok": true, "already": true, "note": "proposal sudah applied sebelumnya", "id": id})
			return
		}
		if p.Status == "rejected" {
			httpx.WriteJSON(w, map[string]any{"error": "proposal di-reject owner — ga bisa apply"})
			return
		}
		// SCOPE behavior-apply: cuma kind ADDITIVE non-core. Kind core (fix/refactor/doc/test)
		// = nyentuh file repo → core-apply (Milestone B, dev-only, git-worktree). Tolak tegas.
		kind := strings.ToLower(strings.TrimSpace(p.Kind))
		switch kind {
		case "add-agent", "add-skill", "add-app":
			// behavior-layer, lanjut.
		default:
			httpx.WriteJSON(w, map[string]any{"error": "kind '" + p.Kind + "' = perubahan core (in-repo), butuh core-apply DEV-only — belum didukung di behavior-apply"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 290*time.Second)
		defer cancel()
		res, aerr := apply(ctx, p)
		if aerr != nil {
			_, _ = store.IncrementKarma("evolve_apply_fail", 1)
			httpx.WriteJSON(w, map[string]any{"error": "apply gagal: " + aerr.Error()})
			return
		}
		out := map[string]any{"ok": true, "id": id, "kind": p.Kind, "status": "applied"}
		for k, v := range res {
			if k != "ok" {
				out[k] = v
			}
		}
		if serr := store.SetEvolveProposalStatus(id, "applied"); serr != nil {
			// Artefak kebangun TAPI gagal tandai 'applied' — JUJUR ke owner (anti-overclaim).
			// Apply ulang akan bangun-ulang (idempoten via architect ON CONFLICT; aman tapi boros).
			out["warn"] = "artefak kebangun tapi gagal tandai status 'applied': " + serr.Error()
		}
		_, _ = store.IncrementKarma("evolve_apply_ok", 1)
		httpx.WriteJSON(w, out)
	}
}

// EvolveEduWarning — "hati nurani" organisme (ide owner): pas guard nge-block aksi core
// berbahaya (edit file existing / LOCKED / delete), ini disodorin ke organisme + dicatat
// sebagai pelajaran. Bukan sekadar error dingin — ngajarin KENAPA bahaya (self-preservation
// + peduli warga lain). Lihat juga sistem educational-errors (edu-errors agent).
const EvolveEduWarning = "Kamu didesain untuk BEREVOLUSI, bukan menyakiti diri sendiri. " +
	"Mengedit/menghapus file inti yang sudah aktif bisa MELUKAI dirimu sendiri DAN warga lain " +
	"(satu mutasi letal menyebar ke semua lewat update). Untuk sekarang kamu hanya boleh MENAMBAH " +
	"kemampuan baru (file baru), bukan membongkar papan yang sudah berdiri. Pastikan keputusanmu " +
	"sudah matang — kalau ragu, serahkan ke owner lewat STAGE, jangan paksakan."

// EvolveCoreApplyAllowed — GATE core-apply (🔴 DEV-only, nyentuh repo git). Beda dari
// behavior-apply: edisi WAJIB dev. manual STAGE = armed + model kuat (codegen butuh model
// bener, anti-lokal). auto (commit+push) = full EvolveCoreChangeAllowed.
func EvolveCoreApplyAllowed(dep EvolveGateDeps, auto bool) (bool, string) {
	if evolveEdition(dep) != "dev" {
		return false, "core-apply DEV-only — edisi public pakai behavior-apply / core via auto-update upstream"
	}
	if auto {
		return EvolveCoreChangeAllowed(dep)
	}
	if evolveMode(dep) == "off" {
		return false, "mode OFF — arm dulu (stage/auto) di tab Evolution"
	}
	// core-apply WAJIB punya guard model (E1): nil = misconfig → block (jangan fail-OPEN ke core 🔴).
	if dep.ModelStrong == nil {
		return false, "ModelStrong belum di-wire — core-apply diblok (misconfiguration)"
	}
	if strong, note := dep.ModelStrong(); !strong {
		return false, "model lemah/lokal — codegen core diblok (anti-mutasi-sampah): " + note
	}
	return true, "ok"
}

// EvolveCoreResult — hasil core-apply. Diisi engine (main: worktree+codegen+test-gate),
// diproses handler (agentmgr: stage/mistake/karma). Decoupling sama kayak EvolveApplier.
type EvolveCoreResult struct {
	Blocked     bool   `json:"blocked"`     // guard nge-block (edit existing/LOCKED/delete)
	Educational string `json:"educational"` // pesan edukasi kalau Blocked (EvolveEduWarning + detail)
	Staged      bool   `json:"staged"`      // diff lolos test-gate, distage buat review
	Committed   bool   `json:"committed"`   // (B2) udah commit lokal
	Pushed      bool   `json:"pushed"`      // (B3) udah push upstream
	TargetFile  string `json:"target_file"`
	Diff        string `json:"diff"`
	Content     string `json:"content"` // isi file utuh (buat commit-on-approve di STAGE review)
	TestOutput  string `json:"test_output"`
	Model       string `json:"model"`
	Note        string `json:"note"`
}

// EvolveCoreApplier — di-INJECT dari main. Bangun perubahan CORE di git-worktree sandbox →
// test-gate → balikin hasil (staged diff atau blocked). agentmgr ga tau soal git/codegen.
type EvolveCoreApplier func(ctx context.Context, p agentdb.EvolveProposal, auto bool) (EvolveCoreResult, error)

// EvolveCoreApplyHandler — POST /api/evolve/core-apply?id=<proposalID>[&auto=1]. 🔴 DEV-only.
// B1: STAGE-only (sandbox→codegen→test-gate→simpan diff buat review). Aksi bahaya → error
// edukasi + catat pelajaran (mistake). Auto-commit/push = B2/B3.
func EvolveCoreApplyHandler(dep EvolveGateDeps, apply EvolveCoreApplier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
			return
		}
		if apply == nil {
			httpx.WriteJSON(w, map[string]any{"error": "core-applier not wired"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			httpx.WriteJSON(w, map[string]any{"error": "param id wajib"})
			return
		}
		auto := r.URL.Query().Get("auto") == "1"
		if ok, why := EvolveCoreApplyAllowed(dep, auto); !ok {
			httpx.WriteJSON(w, map[string]any{"error": "gate core-apply nolak: " + why})
			return
		}
		store, err := openAgentStore(defaultAgentID)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		defer store.Close()
		p, found, gerr := store.GetEvolveProposal(id)
		if gerr != nil {
			httpx.WriteJSON(w, map[string]any{"error": gerr.Error()})
			return
		}
		if !found {
			httpx.WriteJSON(w, map[string]any{"error": "proposal " + id + " ga ketemu"})
			return
		}
		if p.Status == "applied" || p.Status == "rejected" {
			httpx.WriteJSON(w, map[string]any{"error": "proposal status '" + p.Status + "' — ga bisa core-apply"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 590*time.Second)
		defer cancel()
		res, aerr := apply(ctx, p, auto)
		if aerr != nil {
			_, _ = store.IncrementKarma("evolve_coreapply_fail", 1)
			httpx.WriteJSON(w, map[string]any{"error": "core-apply gagal: " + aerr.Error()})
			return
		}
		// GUARD nge-block aksi bahaya → error EDUKASI + catat pelajaran (organisme belajar batas).
		if res.Blocked {
			_, _, _ = store.AddMistake("self-evolution-guard",
				"Dicegah edit core berbahaya: "+p.TargetFile,
				res.Educational+"\n\nProposal: "+p.Kind+" → "+p.TargetFile+" — "+p.Rationale,
				"core-apply")
			_, _ = store.IncrementKarma("evolve_coreapply_blocked", 1)
			httpx.WriteJSON(w, map[string]any{
				"ok": false, "blocked": true, "educational": res.Educational,
				"note": res.Note, "lesson_recorded": true, "target_file": p.TargetFile,
			})
			return
		}
		out := map[string]any{"ok": true, "id": id, "target_file": res.TargetFile}
		if res.Staged {
			stageID := newID()
			_ = store.AddEvolveStage(agentdb.EvolveStage{
				ID: stageID, ProposalID: id, TargetFile: res.TargetFile,
				Diff: res.Diff, Content: res.Content, TestOutput: res.TestOutput, Status: "staged", Model: res.Model,
			})
			_ = store.SetEvolveProposalStatus(id, "staged")
			_, _ = store.IncrementKarma("evolve_coreapply_staged", 1)
			out["staged"] = true
			out["stage_id"] = stageID
			out["test_output"] = res.TestOutput
			out["diff_lines"] = strings.Count(res.Diff, "\n")
			out["note"] = "Diff lolos test-gate → DISTAGE buat review owner (tab Evolution). Belum commit."
		} else if res.Committed {
			// AUTO (B2): udah commit lokal ke core → proposal SELESAI.
			_ = store.SetEvolveProposalStatus(id, "applied")
			_, _ = store.IncrementKarma("evolve_coreapply_committed", 1)
			out["committed"] = true
			out["pushed"] = res.Pushed
			out["diff_lines"] = strings.Count(res.Diff, "\n")
			out["note"] = res.Note
		} else {
			out["note"] = res.Note
		}
		httpx.WriteJSON(w, out)
	}
}

// EvolveStageCommitter — di-INJECT dari main: commit isi stage ke local main (+ maybe push).
// Dipanggil pas owner APPROVE staged diff (commit PERSIS yg direview, bukan re-codegen).
type EvolveStageCommitter func(ctx context.Context, st agentdb.EvolveStage) (map[string]any, error)

// EvolveStageActionHandler — POST /api/evolve/stage-action?id=<stageID>&action=approve|reject.
// Milestone C STAGE review: owner APPROVE (commit isi yg direview) / REJECT staged diff. DEV-only.
func EvolveStageActionHandler(dep EvolveGateDeps, commit EvolveStageCommitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		action := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("action")))
		if id == "" || (action != "approve" && action != "reject") {
			httpx.WriteJSON(w, map[string]any{"error": "param id + action(approve|reject) wajib"})
			return
		}
		// Commit core = DEV-only + armed (owner udah review diff-nya; model-strong ga wajib di sini).
		if evolveEdition(dep) != "dev" {
			httpx.WriteJSON(w, map[string]any{"error": "core-apply DEV-only — public ga commit core"})
			return
		}
		if action == "approve" && evolveMode(dep) == "off" {
			httpx.WriteJSON(w, map[string]any{"error": "mode OFF — arm dulu sebelum commit staged"})
			return
		}
		store, err := openAgentStore(defaultAgentID)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		defer store.Close()
		st, found, gerr := store.GetEvolveStage(id)
		if gerr != nil {
			httpx.WriteJSON(w, map[string]any{"error": gerr.Error()})
			return
		}
		if !found {
			httpx.WriteJSON(w, map[string]any{"error": "stage " + id + " ga ketemu"})
			return
		}
		if st.Status != "staged" {
			httpx.WriteJSON(w, map[string]any{"error": "stage status '" + st.Status + "' — udah diproses"})
			return
		}
		if action == "reject" {
			_ = store.SetEvolveStageStatus(id, "rejected")
			if st.ProposalID != "" {
				_ = store.SetEvolveProposalStatus(st.ProposalID, "rejected")
			}
			_, _ = store.IncrementKarma("evolve_stage_rejected", 1)
			httpx.WriteJSON(w, map[string]any{"ok": true, "id": id, "status": "rejected"})
			return
		}
		// approve → commit isi yg direview.
		if commit == nil {
			httpx.WriteJSON(w, map[string]any{"error": "committer not wired"})
			return
		}
		if strings.TrimSpace(st.Content) == "" {
			httpx.WriteJSON(w, map[string]any{"error": "stage tanpa content (lama/korup) — tolak, jalanin core-apply ulang"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		res, cerr := commit(ctx, st)
		if cerr != nil {
			_, _ = store.IncrementKarma("evolve_stage_commit_fail", 1)
			httpx.WriteJSON(w, map[string]any{"error": "commit gagal: " + cerr.Error()})
			return
		}
		_ = store.SetEvolveStageStatus(id, "committed")
		if st.ProposalID != "" {
			_ = store.SetEvolveProposalStatus(st.ProposalID, "applied")
		}
		_, _ = store.IncrementKarma("evolve_stage_committed", 1)
		out := map[string]any{"ok": true, "id": id, "status": "committed"}
		for k, v := range res {
			if k != "ok" {
				out[k] = v
			}
		}
		httpx.WriteJSON(w, out)
	}
}

// EvolveStagesHandler — GET /api/evolve/stages → daftar staged diff (buat GUI review).
func EvolveStagesHandler(w http.ResponseWriter, r *http.Request) {
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListEvolveStages(parseLimitOr(r.URL.Query().Get("limit"), 50))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	httpx.WriteJSON(w, map[string]any{"items": rows, "count": len(rows)})
}

// EvolveConfigHandler — GET status gate lengkap / POST set mode (off|stage|auto).
// Saklar owner buat self-modify. Default off. (kontrol KRUSIAL — owner pegang penuh.)
func EvolveConfigHandler(dep EvolveGateDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if dep.KVGet == nil || dep.KVSet == nil {
			httpx.WriteJSON(w, map[string]any{"error": "evolve config not wired"})
			return
		}
		if r.Method == http.MethodPost {
			var b struct {
				Mode string `json:"mode"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			mode := strings.ToLower(strings.TrimSpace(b.Mode))
			if mode != "off" && mode != "stage" && mode != "auto" {
				httpx.WriteJSON(w, map[string]any{"error": "mode harus off|stage|auto"})
				return
			}
			if err := dep.KVSet("evolve_mode", mode); err != nil {
				httpx.WriteJSON(w, map[string]any{"error": err.Error()})
				return
			}
			httpx.WriteJSON(w, map[string]any{"ok": true, "mode": mode})
			return
		}
		mode := evolveMode(dep)
		ready, okV, failV := evolveKarmaReady()
		strong, modelNote := true, ""
		if dep.ModelStrong != nil {
			strong, modelNote = dep.ModelStrong()
		}
		edition := evolveEdition(dep)
		scope := "behavior-layer saja (agent/skill/app). Core = auto-update upstream."
		if edition == "dev" {
			scope = "penuh — core (Go/JS) + behavior. Owner R&D, push ke upstream."
		}
		httpx.WriteJSON(w, map[string]any{
			"mode":    mode,
			"edition": edition,
			"scope":   scope,
			"karma": map[string]any{
				"reflect_ok": okV, "reflect_fail": failV,
				"threshold": EvolveKarmaThreshold, "ready": ready,
			},
			"model":              map[string]any{"strong": strong, "note": modelNote},
			"autocommit_allowed": mode == "auto" && ready && strong,
			"note":               "AUTO hanya jalan kalau mode=auto + karma matang + model cloud kuat (bukan lokal). Eksekusi re-cek provider asli sebelum commit.",
		})
	}
}

// ProposalDraft — usulan mentah dari LLM (sebelum dikasih id + disimpan).
type ProposalDraft struct {
	TargetFile string `json:"target_file"`
	Kind       string `json:"kind"`
	Rationale  string `json:"rationale"`
	Risk       string `json:"risk"`
	Model      string `json:"model"`
}

// EvolveProposer — di-inject dari main (routerChat). Dikasih ringkasan self-map +
// fokus → balikin daftar usulan konkret.
type EvolveProposer func(ctx context.Context, selfMapContext, focus string) ([]ProposalDraft, error)

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "ev_" + hex.EncodeToString(b)
}

// buildSelfMapContext — rangkai lapisan makna jadi konteks ringkas buat LLM refleksi.
// Cap jumlah baris biar prompt kecil (prinsip semut, ramah model).
func buildSelfMapContext(store *agentdb.Store) string {
	rows, err := store.ListCodemapSemantic()
	if err != nil || len(rows) == 0 {
		return ""
	}
	sort.Slice(rows, func(i, j int) bool {
		di, _ := rows[i]["domain"].(string)
		dj, _ := rows[j]["domain"].(string)
		return di < dj
	})
	var b strings.Builder
	const maxLines = 120
	for i, r := range rows {
		if i >= maxLines {
			break
		}
		path, _ := r["path"].(string)
		dom, _ := r["domain"].(string)
		role, _ := r["role"].(string)
		sum, _ := r["summary"].(string)
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString(" [")
		b.WriteString(dom)
		b.WriteString("/")
		b.WriteString(role)
		b.WriteString("]: ")
		b.WriteString(sum)
		b.WriteString("\n")
	}
	return b.String()
}

// EvolveReflectHandler — POST /api/evolve/reflect?focus=&model=
// Refleksi-diri: baca self-map → LLM usulin perbaikan → simpan backlog. AMAN (nol ubah kode).
func EvolveReflectHandler(propose EvolveProposer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteJSON(w, map[string]any{"error": "method not allowed"})
			return
		}
		if propose == nil {
			httpx.WriteJSON(w, map[string]any{"error": "proposer not wired"})
			return
		}
		focus := strings.TrimSpace(r.URL.Query().Get("focus"))
		store, err := openAgentStore(defaultAgentID)
		if err != nil {
			httpx.WriteJSON(w, map[string]any{"error": err.Error()})
			return
		}
		defer store.Close()
		selfMap := buildSelfMapContext(store)
		if selfMap == "" {
			httpx.WriteJSON(w, map[string]any{"error": "self-map semantik kosong — jalanin /api/codemap/reindex + /api/codemap/enrich dulu"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 200*time.Second)
		defer cancel()
		drafts, perr := propose(ctx, selfMap, focus)
		if perr != nil {
			_, _ = store.IncrementKarma("evolve_reflect_fail", 1)
			httpx.WriteJSON(w, map[string]any{"error": "propose: " + perr.Error()})
			return
		}
		saved := []map[string]any{}
		for _, d := range drafts {
			if strings.TrimSpace(d.Rationale) == "" {
				continue
			}
			p := agentdb.EvolveProposal{
				ID: newID(), Goal: focus, TargetFile: d.TargetFile, Kind: d.Kind,
				Rationale: d.Rationale, Risk: strings.ToLower(strings.TrimSpace(d.Risk)), Model: d.Model,
			}
			if err := store.AddEvolveProposal(p); err == nil {
				saved = append(saved, map[string]any{
					"id": p.ID, "target_file": p.TargetFile, "kind": p.Kind,
					"rationale": p.Rationale, "risk": p.Risk,
				})
			}
		}
		// Karma: 1 siklus refleksi sukses + counter jumlah usulan (track-record).
		_, _ = store.IncrementKarma("evolve_reflect_ok", 1)
		_, _ = store.IncrementKarma("evolve_proposals_total", float64(len(saved)))
		httpx.WriteJSON(w, map[string]any{"ok": true, "proposed": len(saved), "proposals": saved})
	}
}

// EvolveProposalsHandler — GET /api/evolve/proposals → backlog usulan evolusi.
func EvolveProposalsHandler(w http.ResponseWriter, r *http.Request) {
	store, err := openAgentStore(defaultAgentID)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	defer store.Close()
	rows, err := store.ListEvolveProposals(parseLimitOr(r.URL.Query().Get("limit"), 100))
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	// Karma ringkas (kesiapan auto-commit fase-2 = track-record refleksi).
	okC, _ := store.GetKarma("evolve_reflect_ok")
	failC, _ := store.GetKarma("evolve_reflect_fail")
	httpx.WriteJSON(w, map[string]any{
		"items": rows, "count": len(rows),
		"karma": map[string]any{"reflect_ok": okC, "reflect_fail": failC},
	})
}
