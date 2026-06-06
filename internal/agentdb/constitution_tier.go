// constitution_tier.go — penyesuaian konstitusi PER-TIER.
//
// EXTEND constitution.go (yg LOCKED: "extend via file baru, JANGAN modify ini")
// — di sini NAMBAH method, BUKAN ngubah sacredSeed/seed/sync di file locked.
//
// Kenapa: rule sacred `anti-halu` nyuruh verifikasi pakai brain_search_shared
// (korpus 5jt router). Tapi agent EXTENSION ke-gate dari 5jt — brain-nya di
// FOLDER SENDIRI (brain_search lokal). Kalau konstitusi nyuruh pake tool yg dia
// ga punya → malah MANCING halu / panggilan ke-tolak. Buat extension, rule-nya
// dirapihin: cukup brain_search lokal + web_search.
//
// SEMANGAT anti-halu TETAP UTUH ("verifikasi sebelum ngeklaim, jangan pake tool
// yg ga ada") — justru lebih konsisten buat tier extension.

package agentdb

import "strings"

const (
	// antiHaluSharedFragment — potongan teks DEFAULT (sacredSeed) yg nyebut 5jt.
	antiHaluSharedFragment = "brain_search lokal, brain_search_shared, web_search"
	// antiHaluExtensionFragment — versi extension (brain folder sendiri, no 5jt).
	antiHaluExtensionFragment = "brain_search lokal (brain folder sendiri lo), web_search"
)

// TuneConstitutionForExtension — buat agent EXTENSION: buang sebutan
// brain_search_shared (5jt) dari rule anti-halu. Surgical string-replace —
// robust (kalau teks udah beda/ke-tune, no-op aman) + idempotent. Panggil
// SEBELUM SyncConstitutionSlot biar slot self_prompt ke-update. Return (changed, err).
func (s *Store) TuneConstitutionForExtension() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureConstitutionSchema()

	var rule string
	if err := s.db.QueryRow(`SELECT rule FROM constitution WHERE id='anti-halu'`).Scan(&rule); err != nil {
		return false, err // belum ke-seed / ga ada → skip (caller abaikan err = no-op aman)
	}
	if !strings.Contains(rule, antiHaluSharedFragment) {
		return false, nil // udah di-tune atau teks beda → no-op
	}
	tuned := strings.Replace(rule, antiHaluSharedFragment, antiHaluExtensionFragment, 1)
	if _, err := s.db.Exec(`UPDATE constitution SET rule=? WHERE id='anti-halu'`, tuned); err != nil {
		return false, err
	}
	return true, nil
}
