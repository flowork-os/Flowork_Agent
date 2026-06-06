// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 12 phase 1 accessor (Store.DB read-only handle). API
//   stable: DB() returns *sql.DB. Phase 2 split jadi ReaderDB() untuk
//   strict typing → tambah method baru, JANGAN modify DB().
//
// accessor.go — Section 12 support: expose internal *sql.DB ke caller
// (sandbox.go di internal/tools/). Tetap private package member, cuma
// expose via method.
//
// Anti-misuse: caller wajib HANYA pakai untuk SELECT (read). Write goes
// via Store methods (locked + structured). Phase 2 split jadi ReaderDB()
// untuk lebih strict typing.

package agentdb

import "database/sql"

// DB — return raw *sql.DB. Caller wajib treat as read-only handle. Write
// langsung bypass mutex protection di Store methods — bug risk.
//
// Section 12: tools/sandbox.go pakai untuk query tool_overrides +
// tool_invocations rate counts.
func (s *Store) DB() *sql.DB {
	return s.db
}
