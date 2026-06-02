// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork_Agent
// Locked at: 2026-06-02
// Reason: FASE 4 GATE LULUS — Category Task orchestrator (fan-out crew → copy
//   file → fan-in synthesizer). E2E verified SAHAM: crew (BUY, grounded,
//   bersumber) ngalahin single-agent (loop limit). Phase 2 (crew dari DB/GUI)
//   → extend, jangan rombak core ini.
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

// CrewMember — 1 worker dalam crew.
type CrewMember struct {
	AgentID   string // id agent (hasil spawn template)
	RoleLabel string // deskripsi tugas (buat prompt + report)
}

// Category — definisi 1 kategori task: crew analis + synthesizer.
type Category struct {
	Name        string
	Crew        []CrewMember // analis fan-out (mode sequential di Phase 1)
	Synthesizer string       // agent id yang ambil keputusan (fan-in)
}

// categories — Phase 1 HARDCODED. Phase 2 pindah ke flowork.db + GUI.
var categories = map[string]Category{
	"saham": {
		Name: "Analisa Saham",
		Crew: []CrewMember{
			{AgentID: "saham-fundamental", RoleLabel: "analis fundamental (bisnis, valuasi, prospek)"},
			{AgentID: "saham-keuangan", RoleLabel: "analis laporan keuangan (revenue, laba, utang, arus kas)"},
			{AgentID: "saham-teknikal", RoleLabel: "analis teknikal (tren harga, support/resistance, momentum)"},
		},
		Synthesizer: "saham-sinteser",
	},
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

// Categories — list nama kategori yang kedaftar (buat HTTP info).
func Categories() []string {
	out := make([]string, 0, len(categories))
	for k := range categories {
		out = append(out, k)
	}
	return out
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
//	sharedDir  — path HOST ke shared workspace (mount /shared di guest).
//	category   — key kategori (mis. "saham").
//	input      — subjek (mis. "BBCA").
//	runID      — id run unik (caller stamp; taskflow ga generate waktu sendiri).
//
// Flow: tiap worker di-invoke sequential → diminta tulis ke /shared/tasks/<run>/
// <agent>.md → synthesizer baca file-file itu (on-demand via tools-nya) → vonis.
func RunCategoryTask(ctx context.Context, host Invoker, sharedDir, category, input, runID string) Result {
	res := Result{Category: category, Input: input, RunID: runID}
	cat, ok := categories[strings.ToLower(strings.TrimSpace(category))]
	if !ok {
		res.Err = "kategori ga dikenal: " + category
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

	// ── Fan-out: tiap analis sequential (Phase 1; parallel = Phase 4 nanti) ──
	for _, m := range cat.Crew {
		t0 := time.Now()
		fname := taskFileName(runID, m.AgentID)
		prompt := fmt.Sprintf(
			"[TASK ANALISA SAHAM] Subjek: %s.\n"+
				"Peran lo: %s.\n"+
				"FOKUS tugas lo doang — jangan ngerjain peran analis lain.\n"+
				"Cari data REAL pakai tools (web_search/html_extract) — JANGAN ngarang angka/fakta. "+
				"EFISIEN: MAKSIMAL 2-3 pencarian, terus LANGSUNG simpulin (jangan kebanyakan search).\n"+
				"Kalau ga nemu data, bilang 'data ga ketemu' — jangan maksa.\n"+
				"Habis itu WAJIB tulis hasil analisa lo (ringkas, poin-poin, sebut sumber) via tool "+
				"file_write dengan category=\"%s\" name=\"%s\". Itu langkah TERAKHIR + WAJIB.\n"+
				"Balas singkat aja konfirmasi udah nulis.",
			input, m.RoleLabel, taskFileCategory, fname)

		reply, err := host.InvokeAgentMessage(ctx, m.AgentID, prompt, "taskflow:"+runID)
		workerFile := filepath.Join(sharedDir, m.AgentID, taskFileCategory, fname)
		step := StepResult{
			AgentID:   m.AgentID,
			RoleLabel: m.RoleLabel,
			OutputRef: workerFile,
			Reply:     truncate(reply, 400),
			MS:        time.Since(t0).Milliseconds(),
		}
		if err != nil {
			step.Err = err.Error()
		} else if cerr := copyFile(workerFile, filepath.Join(synthJobDir, fname)); cerr != nil {
			// File analis ga ada / ga ke-copy → synth ga bisa baca. Catat.
			step.Err = "output ga ke-tulis/ke-copy: " + cerr.Error()
		}
		res.Steps = append(res.Steps, step)
	}

	// ── Fan-in: synthesizer baca output crew → 1 keputusan ──
	t0 := time.Now()
	var fileList []string
	for _, m := range cat.Crew {
		fileList = append(fileList, fmt.Sprintf("- category=\"%s\" name=\"%s\"  (%s)",
			taskFileCategory, taskFileName(runID, m.AgentID), m.RoleLabel))
	}
	synthPrompt := fmt.Sprintf(
		"[SYNTHESIZER — AMBIL KEPUTUSAN SAHAM] Subjek: %s.\n"+
			"Tim analis udah nulis hasil ke file-file ini (baca pakai tool file_read, on-demand):\n%s\n"+
			"Baca SEMUA file di atas (file_read per category+name di atas), lalu kasih 1 keputusan "+
			"investasi yang TEGAS:\n"+
			"  KEPUTUSAN: BUY / HOLD / AVOID\n"+
			"  ALASAN: 3-5 poin berbasis data dari analis (sebut dari analis mana).\n"+
			"  RISIKO: 2-3 risiko utama.\n"+
			"Kalau data analis tipis/ga lengkap, bilang jujur + turunin confidence. JANGAN ngarang.",
		input, strings.Join(fileList, "\n"))

	rec, serr := host.InvokeAgentMessage(ctx, cat.Synthesizer, synthPrompt, "taskflow:"+runID)
	res.SynthMS = time.Since(t0).Milliseconds()
	if serr != nil {
		res.Err = "synthesizer: " + serr.Error()
		return res
	}
	res.Recommendation = rec
	return res
}

// copyFile — copy src → dst (overwrite). Error kalau src ga ada (= worker ga
// nulis output). Dipakai mindahin output analis ke dir job synthesizer.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
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
