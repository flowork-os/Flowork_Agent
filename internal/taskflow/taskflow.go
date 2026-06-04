// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-02
// Reason: FASE 4 GATE LULUS — Category Task orchestrator (fan-out crew → copy
//   file → fan-in synthesizer). E2E verified SAHAM: crew (BUY, grounded,
//   bersumber) ngalahin single-agent (loop limit). Phase 2 (crew dari DB/GUI)
//   → extend, jangan rombak core ini.
//   2026-06-03 EXTEND (additif): Category.SynthDirective — kategori non-finansial
//   (YouTube Ops) override format keputusan synth (kosong = default BUY/HOLD/
//   AVOID, backward-compat crypto/saham). Fan-out/fan-in core TIDAK diubah.
//
// taskflow.go — roadmap FASE 4 / design doc Phase 1: Category Task orchestrator.
//
// Konsep (doc/category_task_design.md): MR.FLOW trigger CREW agent fokus (fan-out)
// → tiap worker tulis hasil ke /shared/tasks/<run>/<agent>.md → SYNTHESIZER baca
// on-demand → 1 keputusan (fan-in). Anti-halu: prompt worker kecil + data antar-
// agent lewat FILE (bukan prompt-chaining).
//
// Phase 1 (GATE): crew SAHAM HARDCODED di sini (belum GUI/DB — itu Phase 2).
// Buktiin 1 kategori ngalahin single-agent dulu sebelum generalize.
//
// No import cycle: orchestrator cuma butuh Invoker interface (Host memenuhi).

package taskflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Invoker — kontrak minimal ke kernel host. *kernelhost.Host memenuhi ini
// (InvokeAgentMessage). Pakai interface biar taskflow ga import kernelhost
// (anti cycle) + gampang di-stub buat test.
type Invoker interface {
	InvokeAgentMessage(ctx context.Context, agentID, text, caller string) (string, error)
}

// Recorder — hook OPSIONAL buat persist step ke DB (timeline live, Fase 5).
// nil = ga di-persist (mis. test). StartStep balikin id buat FinishStep.
type Recorder interface {
	StartStep(agentID, role string, idx int) int64
	FinishStep(stepID int64, status, outputRef, errStr string, ms int64)
}

// CrewMember — 1 worker dalam crew.
type CrewMember struct {
	AgentID   string // id agent (hasil spawn template)
	RoleLabel string // deskripsi tugas (buat prompt + report)
}

// Category — definisi 1 kategori task: crew analis + synthesizer.
// Fase 5: di-LOAD dari flowork.db (task_categories/task_agents) oleh caller,
// BUKAN hardcoded lagi. taskflow ga tau soal DB (caller yang rakit Category ini).
type Category struct {
	ID          string
	Name        string
	Crew        []CrewMember // analis fan-out (mode sequential di Phase 1)
	Synthesizer string       // agent id yang ambil keputusan (fan-in)
	// SynthDirective: OPSIONAL. Instruksi format keputusan synthesizer. Kosong =
	// default finansial (BUY/HOLD/AVOID) — jaga backward-compat crypto/saham.
	// Diisi caller buat kategori non-finansial (mis. YouTube Ops).
	SynthDirective string
}

// StepResult — hasil 1 worker.
type StepResult struct {
	AgentID   string `json:"agent_id"`
	RoleLabel string `json:"role_label"`
	OutputRef string `json:"output_ref"` // path file /shared/tasks/<run>/<agent>.md
	Reply     string `json:"reply"`      // ringkas reply RPC (audit)
	Err       string `json:"err,omitempty"`
	MS        int64  `json:"ms"`
}

// Result — hasil 1 run Category Task.
type Result struct {
	Category       string       `json:"category"`
	Input          string       `json:"input"`
	RunID          string       `json:"run_id"`
	RunDir         string       `json:"run_dir"`
	Steps          []StepResult `json:"steps"`
	Recommendation string       `json:"recommendation"` // output synthesizer (BUY/HOLD/AVOID + alasan + risiko)
	SynthMS        int64        `json:"synth_ms"`
	Err            string       `json:"err,omitempty"`
}

// taskFileName — nama file output 1 worker. File tools pakai model category+name
// (name = basename, slash dibuang) → ga bisa subdir /tasks/<run>/. Jadi run+agent
// di-encode ke nama file flat di category "job". Landing: <sharedDir>/job/<name>.
func taskFileName(runID, agentID string) string {
	return "run-" + runID + "__" + agentID + ".md"
}

const taskFileCategory = "job" // category whitelist file tools (lihat file.go)

// RunCategoryTask — orchestrate 1 Category Task end-to-end.
//
//	host       — Invoker (kernelhost.Host).
//	sharedDir  — path HOST ke shared workspace (per-agent: <sharedDir>/<agent>/).
//	cat        — definisi kategori + crew (Fase 5: di-load caller dari flowork.db).
//	input      — subjek (mis. "BBCA").
//	runID      — id run unik (caller stamp; = task_runs.id biar nyambung ke timeline).
//	rec        — Recorder OPSIONAL (persist step ke DB live). nil = skip.
//
// Flow: tiap worker di-invoke sequential → tulis file → host copy ke dir synth →
// synthesizer baca → vonis. Tiap step di-record (StartStep/FinishStep) buat timeline.
func RunCategoryTask(ctx context.Context, host Invoker, sharedDir string, cat Category, input, runID string, rec Recorder) Result {
	res := Result{Category: cat.ID, Input: input, RunID: runID}
	if len(cat.Crew) == 0 {
		res.Err = "crew kosong — tambah analis dulu"
		return res
	}
	if strings.TrimSpace(cat.Synthesizer) == "" {
		res.Err = "synthesizer belum di-set"
		return res
	}
	input = strings.TrimSpace(input)
	if input == "" {
		res.Err = "input kosong"
		return res
	}

	// PENTING: shared workspace itu PER-AGENT (<sharedDir>/<agentID>/job/), bukan
	// global. Jadi file analis ga otomatis ke-baca synthesizer (dir beda). Solusi:
	// host COPY file tiap analis → dir job synthesizer SEBELUM invoke synth.
	synthJobDir := filepath.Join(sharedDir, cat.Synthesizer, taskFileCategory)
	if err := os.MkdirAll(synthJobDir, 0o755); err != nil {
		res.Err = "mkdir synth job dir: " + err.Error()
		return res
	}
	res.RunDir = synthJobDir

	taskName := strings.ToUpper(strings.TrimSpace(cat.Name))
	if taskName == "" {
		taskName = strings.ToUpper(cat.ID)
	}

	// ── Fan-out: tiap analis sequential → ENGINE tulis output (invokeWorker) ──
	for idx, m := range cat.Crew {
		res.Steps = append(res.Steps, invokeWorker(ctx, host, sharedDir, synthJobDir, taskName, input, runID, m, idx, "", rec))
	}

	// ── Fan-in + SELF-HEAL: synth ambil keputusan. Kalau synth deteksi data analis
	// SALAH/ga sesuai subjek → output "RETASK <peran>: <koreksi>" → engine kasih
	// TUGAS ULANG ke worker itu (user ga peduli masalahnya, peduli OUTPUT) → synth
	// ulang. Max maxRetaskRounds biar ga infinite. ──
	synthIdx := len(cat.Crew)
	recommendation, synthMS, serr := invokeSynth(ctx, host, synthJobDir, taskName, input, runID, cat, synthIdx, "synthesizer (ambil keputusan)", rec)
	res.SynthMS = synthMS
	for round := 0; round < maxRetaskRounds && serr == nil; round++ {
		role, instruction, need := parseRetask(recommendation)
		if !need {
			break
		}
		target := findCrewByRole(cat.Crew, role)
		if target == nil {
			break // peran ga ke-kenal → stop (jangan loop)
		}
		// kasih TUGAS ULANG ke worker yang ngaco (dengan koreksi), overwrite output-nya.
		res.Steps = append(res.Steps, invokeWorker(ctx, host, sharedDir, synthJobDir, taskName, input, runID, *target, synthIdx+1+round, instruction, rec))
		// synth ulang dengan data yang udah dikoreksi.
		recommendation, synthMS, serr = invokeSynth(ctx, host, synthJobDir, taskName, input, runID, cat, synthIdx, "synthesizer (ulang setelah tugas ulang)", rec)
		res.SynthMS += synthMS
	}
	if serr != nil {
		res.Err = "synthesizer: " + serr.Error()
	}
	res.Recommendation = recommendation
	return res
}

// RunSolo — BASELINE buat A/B GATE: 1 agent ngerjain SEMUA sendiri (fundamental
// + keuangan + teknikal + keputusan) dalam 1 konteks, tanpa crew/synthesizer.
// Bandingin sama RunCategoryTask buat buktiin multi-agent menang/ga.
func RunSolo(ctx context.Context, host Invoker, agentID, input string) (string, int64) {
	prompt := fmt.Sprintf(
		"[ANALISA SAHAM LENGKAP — SOLO] Subjek: %s.\n"+
			"Kerjain SENDIRI analisa menyeluruh: (1) FUNDAMENTAL (bisnis, valuasi, prospek), "+
			"(2) LAPORAN KEUANGAN (revenue, laba, margin, utang, arus kas), (3) TEKNIKAL "+
			"(tren harga, support/resistance, momentum).\n"+
			"Pake tools riset (web_search/html_extract) cari data REAL — JANGAN ngarang angka, "+
			"sebut sumber. Lalu kasih 1 keputusan TEGAS: BUY/HOLD/AVOID + ALASAN + RISIKO.",
		input)
	t0 := time.Now()
	reply, err := host.InvokeAgentMessage(ctx, agentID, prompt, "taskflow-solo")
	if err != nil {
		return "ERR: " + err.Error(), time.Since(t0).Milliseconds()
	}
	return reply, time.Since(t0).Milliseconds()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
