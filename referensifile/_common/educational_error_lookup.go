package db

// educational_error_lookup.go — runtime lookup untuk pesan error edukatif.
//
// Dipanggil dari titik-titik error di kode (interceptors, registry, dll).
// Query tabel educational_errors di settings DB, gabung message_template +
// evolution_hint, format dengan args.
//
// Fallback: kalau DB error / kode ga ditemukan, return string generik biar
// AI tetep dapet info — sistem TIDAK boleh mogok karena error lookup gagal.
//
// Per Ayah 2026-04-25: doktrin di DB (single source of truth). Tools wajib
// pakai helper ini, bukan fmt.Errorf statis lagi.

import (
	"fmt"
)

// GetEducationalError ambil + format pesan error edukatif untuk error_code.
//
// Workspace = path workspace root (untuk locate flowork-settings.sqlite).
// Code = error code, mis. "ERR_WORKSPACE_NOT_FOUND".
// Args = arguments untuk fmt.Sprintf ke message_template.
//
// Return: string siap pakai (message + "\n\n" + hint). Selalu non-empty —
// fallback ke generic format kalau DB miss.
func GetEducationalError(workspace, code string, args ...any) string {
	db, err := SharedSettings(workspace)
	if err != nil {
		return fallbackEducationalError(code, args...)
	}
	var msgTpl, hint string
	row := db.QueryRow(
		"SELECT message_template, evolution_hint FROM educational_errors WHERE error_code = ?",
		code,
	)
	if err := row.Scan(&msgTpl, &hint); err != nil {
		return fallbackEducationalError(code, args...)
	}
	msg := msgTpl
	if len(args) > 0 {
		msg = fmt.Sprintf(msgTpl, args...)
	}
	return msg + "\n\n" + hint
}

// fallbackEducationalError dipakai kalau DB gak available atau code gak terdaftar.
// Sengaja generic — cukup info supaya AI tau apa yang salah dan code mana yg
// belum di-seed (developer follow-up).
func fallbackEducationalError(code string, args ...any) string {
	if len(args) == 0 {
		return fmt.Sprintf("[%s] (pesan edukasi belum di-seed; cek brain/db/educational_errors_seed.go)", code)
	}
	return fmt.Sprintf("[%s] args=%v (pesan edukasi belum di-seed; cek brain/db/educational_errors_seed.go)", code, args)
}
