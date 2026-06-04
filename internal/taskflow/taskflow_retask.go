// taskflow_retask.go — SELF-HEAL: kalau synth deteksi data analis SALAH/ga sesuai
// subjek (mis. salah ticker BBCA vs BBNI), engine kasih TUGAS ULANG ke worker yang
// ngaco — BUKAN nanya user. Prinsip Mr.Dev: "user ga pernah peduli masalahnya di
// mana, mereka peduli OUTPUT-nya." Jadi sistem benerin sendiri.
//
// Helper di-pisah dari taskflow.go (locked) — invokeWorker / invokeSynth /
// parseRetask / findCrewByRole. Dipakai orkestrasi di RunCategoryTask.

package taskflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxRetaskRounds — batas tugas-ulang biar ga infinite loop (synth bisa terus
// minta retask). 2 ronde cukup: kalau abis 2x masih ngaco, output apa adanya
// (jujur tipis) > gantung selamanya.
const maxRetaskRounds = 2

// invokeWorker — jalanin 1 analis: prompt → invoke → ENGINE tulis output ke file
// (worker dir + synth dir) → record step. `extra` = instruksi koreksi buat retask
// (boleh ""). Balik StepResult.
func invokeWorker(ctx context.Context, host Invoker, sharedDir, synthJobDir, taskName, input, runID string, m CrewMember, idx int, extra string, rec Recorder) StepResult {
	label := m.RoleLabel
	if extra != "" {
		label = m.RoleLabel + " (tugas ulang)"
	}
	var stepID int64
	if rec != nil {
		stepID = rec.StartStep(m.AgentID, label, idx)
	}
	t0 := time.Now()
	fname := taskFileName(runID, m.AgentID)
	prompt := fmt.Sprintf(
		"[TASK %s] Subjek: %s.\n"+
			"Peran lo: %s.\n"+
			"FOKUS tugas lo doang — jangan ngerjain peran analis lain.\n"+
			"Cari data REAL pakai tools (web_search/html_extract) — JANGAN ngarang angka/fakta. "+
			"EFISIEN: MAKSIMAL 2-3 pencarian, terus LANGSUNG simpulin.\n"+
			"Kalau ga nemu data, bilang 'data ga ketemu' — jangan maksa/ngarang.\n"+
			"Tulis hasil analisa lo LANGSUNG DI BALASAN (ringkas, poin-poin, sebut sumber). "+
			"GA USAH file_write — cukup balas teks analisanya.",
		taskName, input, m.RoleLabel)
	if extra != "" {
		prompt += "\n\n⚠️ KOREKSI WAJIB (ini TUGAS ULANG, data sebelumnya salah): " + extra +
			"\nPastikan data PERSIS subjek \"" + input + "\" — JANGAN ketuker entitas/ticker/topik lain."
	}

	reply, err := host.InvokeAgentMessage(ctx, m.AgentID, prompt, "taskflow:"+runID)
	workerFile := filepath.Join(sharedDir, m.AgentID, taskFileCategory, fname)
	step := StepResult{
		AgentID:   m.AgentID,
		RoleLabel: m.RoleLabel,
		OutputRef: workerFile,
		Reply:     truncate(reply, 400),
		MS:        time.Since(t0).Milliseconds(),
	}
	status := "done"
	if err != nil {
		step.Err = err.Error()
		status = "error"
	} else {
		content := strings.TrimSpace(reply)
		if content == "" {
			content = "(analis ga balik output)"
			step.Err = "output kosong dari agent"
			status = "error"
		}
		_ = os.MkdirAll(filepath.Dir(workerFile), 0o755)
		_ = os.WriteFile(workerFile, []byte(content), 0o644)
		_ = os.MkdirAll(synthJobDir, 0o755)
		if werr := os.WriteFile(filepath.Join(synthJobDir, fname), []byte(content), 0o644); werr != nil {
			step.Err = "output ga ke-tulis: " + werr.Error()
			status = "error"
		}
	}
	if rec != nil {
		rec.FinishStep(stepID, status, workerFile, step.Err, step.MS)
	}
	return step
}

// invokeSynth — synth baca SEMUA output analis (inline) → 1 keputusan. Prompt-nya
// nyuruh: kalau data ngaco, output "RETASK <peran>: <koreksi>" (BUKAN nanya user).
// Balik (recommendation, ms, err). Record step sendiri.
func invokeSynth(ctx context.Context, host Invoker, synthJobDir, taskName, input, runID string, cat Category, idx int, label string, rec Recorder) (string, int64, error) {
	var stepID int64
	if rec != nil {
		stepID = rec.StartStep(cat.Synthesizer, label, idx)
	}
	t0 := time.Now()
	var analysisBlock strings.Builder
	for _, m := range cat.Crew {
		data, rerr := os.ReadFile(filepath.Join(synthJobDir, taskFileName(runID, m.AgentID)))
		body := strings.TrimSpace(string(data))
		if rerr != nil || body == "" {
			body = "(ga ada output)"
		}
		fmt.Fprintf(&analysisBlock, "### %s\n%s\n\n", m.RoleLabel, truncate(body, 8000))
	}
	synthDirective := strings.TrimSpace(cat.SynthDirective)
	if synthDirective == "" {
		synthDirective = "kasih 1 keputusan yang TEGAS:\n" +
			"  KEPUTUSAN: BUY / HOLD / AVOID\n" +
			"  ALASAN: 3-5 poin berbasis data dari analis (sebut dari analis mana).\n" +
			"  RISIKO: 2-3 risiko utama."
	}
	synthPrompt := fmt.Sprintf(
		"[SYNTHESIZER — AMBIL KEPUTUSAN %s] Subjek: %s.\n"+
			"Di bawah ini DATA ANALIS LENGKAP & FINAL (udah utuh, bukan potongan). Baca dari sini.\n"+
			"DILARANG KERAS: manggil tool · bilang data 'terputus/incomplete/kurang lengkap' · "+
			"minta user kirim data lagi · nanya balik ke user. **User GA AKAN jawab — lo WAJIB kasih hasil.**\n\n"+
			"%s\n"+
			"OUTPUT lo CUMA 2 kemungkinan:\n"+
			"  (A) Kalau data SALAH SUBJEK (mis. ticker/entitas BEDA dari \"%s\", topik melenceng) → "+
			"baris PALING ATAS tulis PERSIS:\n"+
			"      RETASK <nama peran analis yang salah>: <instruksi cari ulang>\n"+
			"      lalu BERHENTI. Engine kasih tugas ulang otomatis (user ga dilibatin).\n"+
			"  (B) Kalau data SESUAI subjek (walau tipis/ga semua lengkap) → LANGSUNG %s\n"+
			"      Kalau ada yang kurang, PAKAI yang ada + turunin confidence — TAPI TETEP KASIH KEPUTUSAN, "+
			"JANGAN nanya, JANGAN nunda. JANGAN ngarang angka yang ga ada di data.",
		taskName, input, analysisBlock.String(), input, synthDirective)

	recommendation, serr := host.InvokeAgentMessage(ctx, cat.Synthesizer, synthPrompt, "taskflow:"+runID)
	ms := time.Since(t0).Milliseconds()
	status := "done"
	errStr := ""
	if serr != nil {
		status = "error"
		errStr = serr.Error()
	}
	if rec != nil {
		rec.FinishStep(stepID, status, "", errStr, ms)
	}
	return recommendation, ms, serr
}

// parseRetask — cari directive "RETASK <peran>: <instruksi>" di output synth
// (toleran prefix markdown spt "**RETASK ...**"). Balik (role, instruction, true).
func parseRetask(text string) (role, instruction string, ok bool) {
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "*#> "))
		if !strings.HasPrefix(strings.ToUpper(l), "RETASK ") {
			continue
		}
		rest := strings.TrimSpace(l[len("RETASK "):])
		colon := strings.Index(rest, ":")
		if colon <= 0 {
			continue
		}
		role = strings.TrimSpace(strings.Trim(rest[:colon], "*#> "))
		instruction = strings.TrimSpace(strings.Trim(rest[colon+1:], "*#> "))
		if role != "" && instruction != "" {
			return role, instruction, true
		}
	}
	return "", "", false
}

// findCrewByRole — cari worker yang RoleLabel-nya match (case-insensitive,
// exact / saling-contains). nil kalau ga ketemu (→ stop retask, jangan loop).
func findCrewByRole(crew []CrewMember, role string) *CrewMember {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return nil
	}
	for i := range crew {
		rl := strings.ToLower(strings.TrimSpace(crew[i].RoleLabel))
		if rl == role || strings.Contains(rl, role) || strings.Contains(role, rl) {
			return &crew[i]
		}
	}
	return nil
}
