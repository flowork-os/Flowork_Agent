package main

// anchor_noise_ext.go — GROWTH-POINT (NON-frozen) buat ANTI-ANCHOR history.
// Tambah pola reply-GAGAL/HALU/PUNT di sini TANPA buka freeze main.go.
//
// AKAR (owner 2026-06-25, bug history-poisoning): model lokal REGURGITASI jawaban
// gagal-nya SENDIRI dari history. Kebukti live: pas mantok, mr-flow ngarang "error 503
// service unavailable" + nge-punt ScheduleWakeup → jawaban-halu itu masuk `interactions`
// → di-feed balik tiap turn → model nge-ECHO terus tanpa beneran kerja. Error-edukasi
// KEBALIK: yang ke-reinforce malah jawaban NGACO. Owner: "jawaban-halu JANGAN jadi pelajaran."
//
// CARA KERJA: `fetchHistory` (main.go) udah buang exchange yg `isAnchorNoise`==true
// (reply-A gagal + user-Q pasangannya) biar model ga anchor. File ini NYAMBUNG ke
// daftar pola itu lewat init()-append ke `anchorNoisePhrases` (var di main.go). NOL
// edit file frozen. Pola di sini = kelas "fabricated-failure / malformed" (high-confidence
// bad; spesifik biar minim false-positive — JANGAN masukin frasa umum).
//
// CATATAN: ini Layer-1 (channel HISTORY, anti-regurgitasi langsung). Layer-2 (DIGEST —
// jangan masukin interaksi gagal ke cognitive-graph permanen) = follow-up di cognitive_dream.

var extraAnchorNoisePhrases = []string{
	// FABRICATED tool/service failure — model NGARANG error yang ga kejadian (halu).
	"error 503", "kena 503", "503 (service", "service unavailable",
	"tool service", "nolak koneksi gue", "lagi down atau overloaded",
	"service yang lagi mati", "sistemnya nolak",
	// MALFORMED — model NULIS tool_call sebagai TEKS di reply final (bukan call beneran),
	// tanda turn-nya ngaco/ga-tuntas → jangan di-anchor.
	"<tool_call>",
}

func init() {
	// Sambung pola ext ke daftar anti-anchor inti (var di main.go). init() jalan
	// sekali pas startup (tinygo -scheduler=none aman: ga butuh goroutine).
	anchorNoisePhrases = append(anchorNoisePhrases, extraAnchorNoisePhrases...)
}
