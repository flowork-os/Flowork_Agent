// time_awareness.go — SEMUA agent SADAR WAKTU (tanggal/bulan/hari/jam/menit/detik) live, default WIB.
//
// MASALAH (owner 2026-06-23): agent dikasih tugas "cari berita/info terbaru" → suka ngaco kasih yang LAMA,
// karena LLM defaultnya pakai tanggal dari TRAINING (basi), bukan waktu nyata. FIX: inject waktu LIVE ke
// system-prompt SEMUA agent tiap turn (lewat SelfPromptRenderHandler → fetchSelfPrompt agent-template).
//
// BUKAN hardcode: dihitung live dari jam sistem (UTC) + offset. Default WIB (UTC+7). SWITCH:
//   - FLOWORK_TZ_OFFSET_HOURS (mis. "7" WIB, "8" WITA, "9" WIT) · FLOWORK_TZ_LABEL (mis. "WIB").
//
// Mulai dari UTC EKSPLISIT → bener walau timezone host beda/ga di-set.
package agentmgr

import (
	"os"
	"strconv"
	"strings"
	"time"
)

var idWeekdays = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
var idMonths = [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// tzOffsetHours — offset jam dari UTC. Default 7 (WIB). Switch: FLOWORK_TZ_OFFSET_HOURS.
func tzOffsetHours() int {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_TZ_OFFSET_HOURS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= -12 && n <= 14 {
			return n
		}
	}
	return 7
}

// tzLabel — label zona. Default "WIB". Switch: FLOWORK_TZ_LABEL.
func tzLabel() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_TZ_LABEL")); v != "" {
		return v
	}
	return "WIB"
}

// nowLocal — instant sekarang di zona lokal (UTC + offset), dihitung dari UTC eksplisit (TZ-host agnostic).
func nowLocal() time.Time {
	return time.Now().UTC().Add(time.Duration(tzOffsetHours()) * time.Hour)
}

// WIBNowHeader — blok markdown "waktu sekarang" buat di-inject ke system-prompt SEMUA agent tiap turn.
// Live + lengkap (hari, tanggal, bulan, tahun, jam:menit:detik). Anti berita-basi.
func WIBNowHeader() string {
	n := nowLocal()
	lbl := tzLabel()
	stamp := idWeekdays[int(n.Weekday())] + ", " + strconv.Itoa(n.Day()) + " " + idMonths[int(n.Month())] +
		" " + strconv.Itoa(n.Year()) + ", " + n.Format("15:04:05") + " " + lbl
	return "# WAKTU SEKARANG (" + lbl + ")\n" +
		"Sekarang **" + stamp + "** (live, bukan dari ingatan/training). Buat cari berita/info/rilis TERBARU, " +
		"patok ke tanggal ini — cari yg dekat hari ini, jangan kasih yg basi.\n\n"
}
