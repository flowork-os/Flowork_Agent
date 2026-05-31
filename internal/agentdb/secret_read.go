// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Read-only secret getter (plug-in, ga sentuh agentdb.go locked).
//   Dipakai host buat notify Telegram owner.
//
// secret_read.go — plug-in: baca nilai secret per-agent dari host (read-only).
// Terpisah dari agentdb.go (LOCKED). Dipakai host buat ambil TELEGRAM_BOT_TOKEN
// + TELEGRAM_ALLOWED_CHATS waktu notify owner (mis. codescan critical finding).

package agentdb

import "database/sql"

// GetSecretValue — value secret by key. Empty string kalau ngga ada.
func (s *Store) GetSecretValue(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v string
	err := s.db.QueryRow(`SELECT v FROM secrets WHERE k = ?`, k).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}
