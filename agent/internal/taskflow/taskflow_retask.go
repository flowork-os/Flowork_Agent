// 🔒 FROZEN GROUP-CORE · Repo: https://github.com/flowork-os/Flowork-OS · Owner: Aola Sahidin (Mr.Dev)
// ⛔ WAJIB sebelum ngedit file ini: BACA /home/mrflow/Documents/FLowork_os/lock/group.md
//    (cara kerja group, filtur, cara bikin group, CABANG *_ext.go). File ini BEKU (chattr +i +
//    hash KERNEL_FREEZE.md). Filtur baru → masuk *_ext.go (RegisterExecStrategy /
//    RegisterGroupSyncHook) atau DATA (Category/Directive). JANGAN buka file beku ini.
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

// maxSynthGuardRetries — berapa kali engine paksa-ulang synth yang NANYA/NUNDA user
// (confabulate "data kurang/analis belum") sebelum nyerah. 2 cukup: haiku biasanya
// nurut di retry-1; kalau abis 2x masih ngeyel, output apa adanya > gantung.
const maxSynthGuardRetries = 2

// invokeWorker — jalanin 1 analis: prompt → invoke → ENGINE tulis output ke file
// (worker dir + synth dir) → record step. `extra` = instruksi koreksi buat retask
// (boleh ""). Balik StepResult.
func invokeWorker(ctx context.Context, host Invoker, sharedDir, synthJobDir, taskName, input, runID string, m CrewMember, idx int, extra, workerDirective string, rec Recorder) StepResult {
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
	// Directive cara kerja worker. Default = analysis-shaped (cari data REAL,
	// jangan ngarang) buat kategori finansial. workerDirective != "" = override
	// per-kategori (mis. zodiak KREATIF: ngarang ramalan MEMANG tugasnya). Mirror
	// pola SynthDirective — additif, backward-compat (kosong = perilaku lama).
	directive := "Cari data REAL pakai tools (web_search/html_extract) — JANGAN ngarang angka/fakta. " +
		"EFISIEN: MAKSIMAL 2-3 pencarian, terus LANGSUNG simpulin.\n" +
		"Kalau ga nemu data, bilang 'data ga ketemu' — jangan maksa/ngarang."
	if d := strings.TrimSpace(workerDirective); d != "" {
		directive = d
	}
	prompt := fmt.Sprintf(
		"[TASK %s] Subjek: %s.\n"+
			"Peran lo: %s.\n"+
			"FOKUS tugas lo doang — jangan ngerjain peran analis lain.\n"+
			"%s\n"+
			"Tulis hasil lo LANGSUNG DI BALASAN (ringkas, poin-poin). "+
			"GA USAH file_write — cukup balas teks-nya.",
		taskName, input, m.RoleLabel, directive)
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
	// Framing EKSPLISIT: kasih nomor "i/n" + header/footer tegas biar synth (haiku)
	// GA confabulate "analis belum masuk". Blok yang nihil di-label SEBAGAI HASIL
	// FINAL (udah jalan, ga nemu) — bukan "belum jalan" (akar bug run#37: teknikal
	// "DATA TIDAK DITEMUKAN" disangka synth = analis belum lapor → nunggu).
	var analysisBlock strings.Builder
	n := len(cat.Crew)
	fmt.Fprintf(&analysisBlock, "═══ OUTPUT FINAL DARI %d/%d ANALIS — SEMUA SUDAH SELESAI & LAPOR ═══\n\n", n, n)
	for i, m := range cat.Crew {
		data, rerr := os.ReadFile(filepath.Join(synthJobDir, taskFileName(runID, m.AgentID)))
		body := strings.TrimSpace(string(data))
		note := ""
		if rerr != nil || body == "" || body == "(analis ga balik output)" {
			body = "Analis ini SUDAH dijalankan — hasilnya: tidak menemukan data."
			note = "  [HASIL: nihil — ini temuan FINAL analis, BUKAN 'belum jalan'. Putuskan pakai analis lain.]"
		}
		fmt.Fprintf(&analysisBlock, "──── ANALIS %d/%d — %s%s ────\n%s\n\n", i+1, n, m.RoleLabel, note, truncate(body, 8000))
	}
	fmt.Fprintf(&analysisBlock, "═══ HABIS — TIDAK ADA ANALIS LAIN. JANGAN tunggu / sebut analis yang \"belum\". ═══\n")
	synthDirective := strings.TrimSpace(cat.SynthDirective)
	if synthDirective == "" {
		synthDirective = "kasih 1 keputusan yang TEGAS:\n" +
			"  KEPUTUSAN: BUY / HOLD / AVOID\n" +
			"  ALASAN: 3-5 poin berbasis data dari analis (sebut dari analis mana).\n" +
			"  RISIKO: 2-3 risiko utama."
	}
	synthPrompt := fmt.Sprintf(
		"[SYNTHESIZER — AMBIL KEPUTUSAN %s] Subjek: %s.\n"+
			"Di bawah ini DATA ANALIS LENGKAP, UTUH & FINAL. Ini SEMUA data yang ada, dan memang CUKUP buat mutusin.\n"+
			"⚠️ PENTING soal format: teks analis BISA mengandung '…', '...', tabel ringkas, atau baris yang "+
			"keliatan kepotong — itu GAYA NULIS analis (ringkas), BUKAN tanda data hilang. JANGAN tafsirin "+
			"'…' atau tabel pendek sebagai 'data terputus/belum utuh'. Anggap TIAP blok analis = SELESAI & FINAL.\n"+
			"DILARANG MUTLAK (kalau dilanggar: output DITOLAK engine = GAGAL TOTAL = user nerima ERROR): "+
			"manggil tool · bilang data 'terputus/incomplete/kurang lengkap/belum utuh' · minta/nunggu data lagi · "+
			"nanya atau minta klarifikasi ke user · nunda/nahan keputusan · "+
			"nyebut/nungguin analis yang 'belum' (SEMUA analis di atas UDAH lapor — yang nulis 'tidak ada data' "+
			"itu HASIL FINAL-nya, putuskan pakai analis lain + turunin confidence). "+
			"**User GA AKAN jawab — lo WAJIB commit hasil SEKARANG.**\n\n"+
			"%s\n"+
			"OUTPUT lo CUMA 2 kemungkinan:\n"+
			"  (A) HANYA kalau data SALAH SUBJEK (ticker/entitas BEDA dari \"%s\", topik melenceng — "+
			"BUKAN sekadar 'kurang lengkap') → baris PALING ATAS tulis PERSIS:\n"+
			"      RETASK <nama peran analis yang salah>: <instruksi cari ulang>\n"+
			"      lalu BERHENTI. Engine kasih tugas ulang otomatis (user ga dilibatin).\n"+
			"  (B) SELAIN itu (data sesuai subjek, walau tipis) → LANGSUNG %s\n"+
			"      Data tipis? PAKAI yang ada + turunin confidence — TAPI TETEP KASIH KEPUTUSAN TEGAS, "+
			"JANGAN nanya, JANGAN nunda, JANGAN minta data. JANGAN ngarang angka yang ga ada di data.\n⚠️ OUTPUT LANGSUNG ISI FINAL (format sesuai directive). DILARANG kalimat pembuka/sapaan/basa-basi (mis. 'Baik', 'Oke', nyebut nama user, 'data sudah final/utuh', 'mari kita susun', 'berdasarkan analisis'). MULAI dari konten hasil, titik.",
		taskName, input, analysisBlock.String(), input, synthDirective)

	recommendation, serr := host.InvokeAgentMessage(ctx, cat.Synthesizer, synthPrompt, "taskflow:"+runID)
	// GUARD anti-nanya: haiku KADANG masih confabulate "data terputus / analis belum"
	// lalu nanya/nunda user, walau prompt udah ngelarang. Kalau output keliatan
	// NANYA/NUNDA (DAN bukan RETASK yang sah), invoke ULANG dengan teguran keras →
	// paksa commit. Loop sampai maxSynthGuardRetries (haiku bisa ngeyel 1x). Sengaja
	// di sini (bukan loop retask di taskflow.go yang LOCKED) biar self-contained.
	for attempt := 0; serr == nil && attempt < maxSynthGuardRetries; attempt++ {
		if _, _, isRetask := parseRetask(recommendation); isRetask || !looksLikeAskingUser(recommendation) {
			break
		}
		fmt.Fprintf(os.Stderr, "[taskflow] synth %s run=%s NANYA/NUNDA user (attempt %d) → retry-paksa commit\n",
			cat.Synthesizer, runID, attempt+1)
		hard := synthPrompt +
			"\n\n──────────\n🚫 JAWABAN LO BARUSAN NANYA / NUNDA / SEBUT 'analis belum' / MINTA DATA — itu " +
			"DILARANG MUTLAK & DITOLAK engine. SEMUA analis (lihat 1/" + fmt.Sprint(len(cat.Crew)) + " s/d " +
			fmt.Sprint(len(cat.Crew)) + "/" + fmt.Sprint(len(cat.Crew)) + ") UDAH lapor di atas, ga ada yang ditunggu. " +
			"SEKARANG ULANG: commit KEPUTUSAN final pakai data apa adanya, TANPA satu pun pertanyaan/penundaan. " +
			"Mulai LANGSUNG dari baris keputusan."
		retry, rerr := host.InvokeAgentMessage(ctx, cat.Synthesizer, hard, "taskflow:"+runID)
		if rerr != nil || strings.TrimSpace(retry) == "" {
			break
		}
		recommendation = retry
	}
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

// looksLikeAskingUser — deteksi synth yang NANYA / NUNDA / MINTA DATA ke user
// (confabulate "data kurang") instead of commit keputusan. Dipakai GUARD di
// invokeSynth buat retry-paksa. Sengaja fokus ke sinyal KUAT biar minim
// false-positive — lagian false-positive cuma rugi 1 retry yang TETEP ngehasilin
// keputusan, jadi aman ke arah "lebih tegas".
func looksLikeAskingUser(text string) bool {
	t := strings.ToLower(text)
	needles := []string{
		"minta klarifikasi", "klarifikasi singkat", "mohon kirim", "tolong kirim",
		"tunggu data", "nunggu data", "kirim data", "kirimkan data", "butuh data tambahan",
		"data belum utuh", "data belum lengkap", "belum lengkap", "data terputus",
		"data terpotong", "jangan synthesize", "jangan synthesiz", "hold jangan",
		"belum bisa putuskan", "belum bisa ambil keputusan", "ga bisa putusin",
		"apakah saya synthesize", "apakah lo mau", "apakah anda", "mau saya lanjut",
		"minta data", "data lengkap dulu", "sebelum saya putuskan",
	}
	for _, n := range needles {
		if strings.Contains(t, n) {
			return true
		}
	}
	return false
}
