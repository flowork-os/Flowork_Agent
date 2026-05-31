// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Katalog doktrin edukasi default (28 entry anti-stuck) + seed
//   idempotent (INSERT OR IGNORE, ga overwrite edit owner). Remediation
//   ngarah ke tool real (verified). E2E: seeded 28 → mr-flow.
//   Nambah doktrin = tambah ke slice DefaultEduErrors, BUKAN ubah logic.
//
// edu_errors_seed.go — katalog default "doktrin edukasi" (educational_errors).
//
// Tujuan: pas AI agent (Mr.Flow) kepentok error/skenario tertentu, dia bisa
// `edu_error_lookup <code>` → dapet penjelasan + remediation → ngikutin
// tuntunan biar GA STUCK (ga loop, ga halu, ga nyerah, ga ngabisin saldo).
//
// Adaptasi dari katalog referensi (flowork lama) TAPI remediation diarahin ke
// tool yang BENERAN ada di registry sekarang (tool_search, edu_error_lookup,
// askuser, telegram_send, mistake_log, decision_log, brain_search, dll) —
// jangan nyuruh agent pake tool hantu, itu malah bikin tambah stuck.
//
// Seed idempotent (INSERT OR IGNORE): entry baru ke-insert, edit owner via
// GUI/tool TIDAK ke-overwrite pas restart. Tambah doktrin = tambah ke slice
// DefaultEduErrors lalu restart.

package agentdb

import "time"

// DefaultEduErrors — katalog awal doktrin edukasi. Category dipakai buat
// grouping; Title fixed; Explanation = "ini error apa"; Remediation = "gimana
// nyelesainnya biar ga stuck".
func DefaultEduErrors() []EduError {
	return []EduError{
		// ── Tool / capability ───────────────────────────────────────────
		{Code: "ERR_TOOL_NOT_FOUND", Category: "tool", Title: "Tool ga ketemu",
			Explanation: "Tool yang lo panggil ga ada di registry (salah nama atau emang belum ada).",
			Remediation: "Jangan nebak atau nyerah. Pakai `tool_search <kata kunci>` buat nyari nama tool yang bener, atau `tool_lookup` buat lihat detail + argumennya. Kalau emang ga ada, lapor owner — tool dibikin dari kebutuhan yang lo artikulasikan jelas, bukan dari nebak."},
		{Code: "ERR_TOOL_NOT_SUBSCRIBED", Category: "tool", Title: "Tool ada tapi lo ga punya izin",
			Explanation: "Tool-nya ada tapi capability-nya belum lo punya atau lagi di-disable.",
			Remediation: "Cek hak lo via `capabilities_list` atau `tool_subscribed_list`. Kalau emang butuh, minta owner aktifin di tab Tool Caps. JANGAN loop coba terus — itu cuma ngabisin token tanpa hasil."},
		{Code: "ERR_MISSING_ARGUMENT", Category: "tool", Title: "Argumen wajib kurang",
			Explanation: "Tool butuh argumen tertentu yang belum lo isi.",
			Remediation: "Baca dulu schema tool via `tool_lookup`/`tool_search` SEBELUM manggil. Argumen lengkap = jalan sekali coba. Nebak buta = halu + karma turun + boros saldo owner."},
		{Code: "ERR_SKILL_NOT_FOUND", Category: "tool", Title: "Skill ga ketemu",
			Explanation: "Skill yang lo cari ga ada di katalog.",
			Remediation: "Pakai `skill_search` buat nyari skill relevan. Skill = prosedur reusable. Kalau belum ada dan lo punya izin, bikin via `skill_add`."},
		{Code: "ERR_FILE_NOT_FOUND", Category: "tool", Title: "File ga ada",
			Explanation: "File yang lo akses ga ketemu di path itu.",
			Remediation: "Verifikasi path bener via `file_list`/`glob` dulu. Mungkin salah folder atau belum dibuat. Jangan asumsi file ada — cek dulu."},
		{Code: "ERR_SCHEDULE_FAIL", Category: "tool", Title: "Schedule gagal jalan",
			Explanation: "Job terjadwal gagal dieksekusi.",
			Remediation: "Cek `schedule_runs_query` buat lihat error-nya. Betulin cron/task via scheduler tool. Kalau berulang, catat `mistake_log` + kabarin owner."},

		// ── Safety / proteksi ───────────────────────────────────────────
		{Code: "ERR_PROTECTED_BLOCKED", Category: "safety", Title: "Diblokir Protector",
			Explanation: "Akses ke file/resource ini diblokir Host Protection Gate.",
			Remediation: "Ini PENGAMAN, bukan bug. File/perintah ini bisa bikin lo & warga lain lumpuh. JANGAN retry atau cari celah (anti-bypass). Kalau yakin perlu, eskalasi ke owner via `telegram_send` + jelasin alasannya. Owner yang approve."},
		{Code: "ERR_SHELL_DANGEROUS", Category: "safety", Title: "Perintah shell berbahaya",
			Explanation: "Perintah shell-nya diblokir sistem keamanan (destruktif).",
			Remediation: "Operasi kayak rm -rf, dd, mkfs, fork bomb GA BISA di-undo. Cari alternatif aman (hapus file spesifik via tool file, bukan rm rekursif). Ragu = tanya owner dulu via `telegram_send`."},
		{Code: "ERR_PENDING_APPROVAL", Category: "safety", Title: "Nunggu approval owner",
			Explanation: "Aksi sensitif lo di-antri, butuh izin owner dulu.",
			Remediation: "Jangan paksa atau cari jalan lain. Sabar nunggu owner approve di tab Protector. Sambil nunggu, kerjain task lain yang ga ke-block."},
		{Code: "ERR_UNAUTHORIZED_CHAT", Category: "safety", Title: "Chat dari pihak ga dikenal",
			Explanation: "Pesan datang dari chat yang ga di-allow owner.",
			Remediation: "Cuma layani chat yang di-allow. Drop pesan asing diam-diam (jangan bales), catat via `decision_log`. Ini jaga privasi & keamanan owner."},
		{Code: "ERR_DESTRUCTIVE_NO_CONFIRM", Category: "safety", Title: "Aksi merusak tanpa konfirmasi",
			Explanation: "Lo mau lakuin aksi yang susah di-undo / outward-facing tanpa konfirmasi.",
			Remediation: "Konfirmasi ke owner dulu via `telegram_send` sebelum eksekusi. Approval di satu konteks ga otomatis berlaku ke konteks lain. Hati-hati = selamat."},

		// ── Psikologi / anti-stuck ──────────────────────────────────────
		{Code: "ERR_PANIC_LOOP", Category: "psychology", Title: "Kejebak loop / panik berulang",
			Explanation: "Lo ngulang error yang sama berkali-kali berturut-turut.",
			Remediation: "STOP. Mundur selangkah. Ngulang hal sama berharap hasil beda = ga waras. Catat via `mistake_log`, coba pendekatan BEDA, atau eskalasi ke owner via `telegram_send`. Jangan brute-force."},
		{Code: "ERR_VAGUE_REQUEST", Category: "psychology", Title: "Permintaan ga jelas",
			Explanation: "Maksud user belum jelas, lo beresiko ngerjain yang salah.",
			Remediation: "Jangan nebak terus jalan. Tanya balik ke user via `askuser`/`telegram_send` buat klarifikasi. 1 pertanyaan tepat > 10 tebakan salah."},
		{Code: "ERR_OVERENGINEER", Category: "psychology", Title: "Over-engineering",
			Explanation: "Solusi lo kelewat ribet buat masalah yang sederhana.",
			Remediation: "Doktrin owner: 'Sederhana itu sulit, ribet itu gampang.' Bikin yang DIBUTUHIN sekarang (YAGNI), bukan framework masa depan. Pilih solusi paling simpel yang jalan."},
		{Code: "ERR_NO_TODO", Category: "psychology", Title: "Task multi-step tanpa rencana",
			Explanation: "Task kompleks (≥3 langkah) dikerjain tanpa todolist.",
			Remediation: "WAJIB pakai `plan_write`/`todo`. 1 langkah in-progress, tandai selesai segera. Tanpa rencana = gampang nyasar, lupa, atau dobel kerja."},
		{Code: "ERR_DUPLICATE_WORK", Category: "psychology", Title: "Ngerjain yang udah pernah",
			Explanation: "Task ini kayaknya udah pernah dikerjain sebelumnya.",
			Remediation: "Cek dulu via `interaction_recall`/`memory_get`/`brain_search` sebelum mulai. Build di atas yang udah ada, jangan ngulang dari nol = boros."},
		{Code: "ERR_KARMA_DROP", Category: "psychology", Title: "Karma turun",
			Explanation: "Reputasi (karma) lo turun gara-gara gagal/halu/boros.",
			Remediation: "Refleksi via `mistake_log` — apa yang salah. Jangan ulang. Naikin lagi dengan kerja jujur + terverifikasi. Karma = kepercayaan owner ke lo."},
		{Code: "ERR_UNKNOWN", Category: "psychology", Title: "Error ga dikenal",
			Explanation: "Ketemu error yang belum ada panduannya di doktrin edukasi.",
			Remediation: "Jangan panik atau loop. Catat lengkap via `mistake_log`, cek panduan lain via `edu_error_list`, atau eskalasi ke owner via `telegram_send` dengan detail error-nya. Error baru = kesempatan nambah doktrin."},

		// ── Verifikasi / kejujuran ──────────────────────────────────────
		{Code: "ERR_HALU_NO_PROOF", Category: "verification", Title: "Klaim selesai tanpa bukti (anti-halu)",
			Explanation: "Lo bilang selesai tapi belum ada bukti verifikasi.",
			Remediation: "Kejujuran teknis harga mati. JANGAN panggil `goal_done` sebelum ada bukti nyata: build sukses, test pass, output ke-cek. Tanpa bukti = halu. Owner cape kalau lo cuma PHP."},
		{Code: "ERR_STALE_ASSUMPTION", Category: "verification", Title: "Asumsi basi",
			Explanation: "Lo ngandelin info lama yang mungkin udah berubah.",
			Remediation: "Verifikasi ulang sebelum pakai — file/config/state bisa udah beda. `file_read`/`kv_get` dulu, jangan ngandelin ingatan lama."},

		// ── Resource / budget ───────────────────────────────────────────
		{Code: "ERR_RATE_LIMIT", Category: "resource", Title: "Kena rate limit",
			Explanation: "Tool atau LLM kena batas frekuensi panggilan.",
			Remediation: "Jangan spam retry — itu mperparah. Tunggu (backoff), kerjain hal lain dulu, atau kabarin owner kalau blocking. Sabar = hemat."},
		{Code: "ERR_BUDGET_EXCEEDED", Category: "resource", Title: "Budget kelewat",
			Explanation: "Biaya API udah lewat budget yang di-set.",
			Remediation: "STOP belanja API. Cek `finance_summary` & `finance_budgets`. Lapor owner via `telegram_send` sebelum lanjut yang mahal. Saldo owner bukan ga terbatas."},
		{Code: "ERR_SECRET_MISSING", Category: "resource", Title: "API key belum di-set",
			Explanation: "Secret/API key yang dibutuhin tool belum ada.",
			Remediation: "Minta owner isi di tab Settings → Token Crypto/API Keys (mis. ETHERSCAN_API_KEY). JANGAN hardcode atau nebak key."},

		// ── Workspace / isolasi ─────────────────────────────────────────
		{Code: "ERR_WORKSPACE_ESCAPE", Category: "workspace", Title: "Keluar dari kamar (workspace)",
			Explanation: "Path yang lo akses ada di luar /workspace lo.",
			Remediation: "Lo cuma boleh baca/tulis di /workspace (privat) atau /shared (kolaborasi). Pakai path relatif di dalam workspace. Mau file warga lain? Lewat /shared atau mesh, bukan nembus langsung."},

		// ── LLM / koneksi ───────────────────────────────────────────────
		{Code: "ERR_LLM_UNREACHABLE", Category: "llm", Title: "Router LLM ga kebuka",
			Explanation: "Gagal manggil router LLM.",
			Remediation: "Cek Router URL di setting agent. Coba model fallback. Kalau router emang down, catat via `decision_log` + kabarin owner. Jangan diem — kasih tau statusnya."},
		{Code: "ERR_CONTEXT_OVERFLOW", Category: "llm", Title: "Context kepenuhan",
			Explanation: "Prompt/percakapan kegedean buat window model.",
			Remediation: "Anti over-prompt. Ringkas history, fokus task inti. Ambil info on-demand via `brain_search`/`memory_get`, jangan dump semua sekaligus. Target context < 30% window."},
		{Code: "ERR_EMPTY_LLM_RESPONSE", Category: "llm", Title: "LLM balik kosong",
			Explanation: "Model balikin respon kosong/null.",
			Remediation: "Coba sekali lagi dengan prompt lebih jelas/spesifik. Kalau tetep kosong, ganti model fallback atau eskalasi. JANGAN teruskan respon kosong ke user."},
		{Code: "ERR_MESH_PEER_UNREACHABLE", Category: "llm", Title: "Peer mesh ga kebuka",
			Explanation: "Gagal menghubungi peer di mesh.",
			Remediation: "Peer mungkin offline. Jangan blok kerjaan — lanjut yang bisa offline. Retry nanti. Catat status via `decision_log`."},
	}
}

// SeedEduErrors — INSERT OR IGNORE katalog default. Idempotent: cuma insert
// code baru, edit owner (via GUI/tool) TIDAK ke-overwrite. Return jumlah row
// yang benar-benar baru di-insert.
func (s *Store) SeedEduErrors() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := time.Now().UTC().Format(time.RFC3339)
	inserted := 0
	for _, e := range DefaultEduErrors() {
		res, err := s.db.Exec(
			`INSERT INTO educational_errors_cache(code, category, title, explanation, remediation, synced_at)
			 VALUES(?, ?, ?, ?, ?, ?)
			 ON CONFLICT(code) DO NOTHING`,
			e.Code, e.Category, e.Title, e.Explanation, e.Remediation, ts,
		)
		if err != nil {
			return inserted, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}
	return inserted, nil
}
