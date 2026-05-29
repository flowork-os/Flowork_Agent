package brain

// record_chat.go — helper untuk insert chat interaction ke `recordings`
// table dari kernel chat path.
//
// Konteks audit Ayah 2026-05-06: "REC. 24H = 0" walaupun chat aktif.
// Root cause: tabel `recordings` cuma di-feed `brain/proxy/proxy.go.recorder`
// yang dipake `cmd/flowork-watcher,auditor,kreator` (background process).
// Kernel chat path lewat `/api/brain/v2/cache-record` (post LLM call dari
// kernel) ngga insert recording → training data ngga tumbuh dari interaksi
// chat normal Ayah.
//
// Helper ini di-call dari cache-record handler supaya recording terisi
// otomatis pasca tiap LLM call. Append-only (FQP-12) — INSERT only, ngga
// pernah UPDATE/DELETE row history.

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
)

// RecordChat insert satu chat interaction ke `recordings` table sebagai
// training data mentah. Fire-and-forget: caller (handler HTTP) ngga harus
// block kalau insert fail — log ke stderr lewat caller defer.
//
// Args:
//   - query: prompt user (raw text)
//   - response: LLM completion
//   - model: model identifier (mis. "qwen2.5-3b" / "openrouter/anthropic/claude-3.5")
//   - warga: agent identifier dari kernel (e.g. "merpati", "aksara"). Kosong = anonymous chat.
//   - inputTokens, outputTokens: usage stats dari LLM response.Usage. Pass 0
//     kalau caller belum punya stat — Health tab akan compute cost dari
//     model_pool prices kalau >0.
//
// 2026-05-06: signature extended dengan token counts (sebelumnya hardcoded
// 0/0 → Health Cost per merpati selalu \$0 misleading).
func RecordChat(db *sql.DB, query, response, model, warga string, inputTokens, outputTokens int) error {
	if query == "" || response == "" {
		return nil // skip empty interactions (idempotent no-op)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(query)))
	_, err := db.Exec(`
		INSERT INTO recordings (prompt_hash, prompt, response, model, input_tokens, output_tokens, tool_calls, agent)
		VALUES (?, ?, ?, ?, ?, ?, '[]', ?)`,
		hash, query, response, model, inputTokens, outputTokens, warga,
	)
	if err != nil {
		return fmt.Errorf("brain.RecordChat: insert: %w", err)
	}
	return nil
}
