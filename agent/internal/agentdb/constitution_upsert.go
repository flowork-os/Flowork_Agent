// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
package agentdb

import "time"

// constitution_upsert.go — UpsertConstitution: tambah/timpa 1 aturan konstitusi
// (custom, by id). Idempotent. Dipakai seed self-evolution buat nanam aturan
// misi/roh Flowork ke DNA tiap otak dewan (always-inject) → council SELALU tahu
// tujuan/roh, evolusi sejalan misi. ensureConstitutionSchema (locked) ga disentuh —
// dipanggil dari sini biar tabel pasti ada.
func (s *Store) UpsertConstitution(id, rule string, amplitude int, sacred, alwaysInject bool, lens string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureConstitutionSchema()
	sa, ai := 0, 0
	if sacred {
		sa = 1
	}
	if alwaysInject {
		ai = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO constitution(id, rule, amplitude, sacred, always_inject, lens, created_at)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET rule=excluded.rule, amplitude=excluded.amplitude,
		   sacred=excluded.sacred, always_inject=excluded.always_inject, lens=excluded.lens`,
		id, rule, amplitude, sa, ai, lens, now,
	)
	return err
}
