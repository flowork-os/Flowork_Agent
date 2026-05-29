package proxy

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"github.com/teetah2402/flowork/internal/provider"
)

// Recorder menyimpan training data ke SQLite.
type Recorder struct {
	db *sql.DB
}

// NewRecorder membuat recorder baru.
func NewRecorder(db *sql.DB) *Recorder {
	return &Recorder{db: db}
}

// RecordInteraction menyimpan satu interaksi prompt-response ke recordings table.
// Ini adalah training data mentah yang bisa dipakai untuk fine-tune model lokal.
func (r *Recorder) RecordInteraction(prompt, response, model string, inputTokens, outputTokens int, toolCalls []provider.ToolCall, agent string) error {
	if prompt == "" || response == "" {
		return nil // skip empty interactions
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))

	toolCallsJSON := "[]"
	if len(toolCalls) > 0 {
		if data, err := json.Marshal(toolCalls); err == nil {
			toolCallsJSON = string(data)
		}
	}

	_, err := r.db.Exec(`
		INSERT INTO recordings (prompt_hash, prompt, response, model, input_tokens, output_tokens, tool_calls, agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		hash, prompt, response, model, inputTokens, outputTokens, toolCallsJSON, agent,
	)
	if err != nil {
		return fmt.Errorf("record interaction: %w", err)
	}
	return nil
}

// RecordToolCall menyimpan pattern tool call untuk learning.
// Setelah cukup data, brain bisa prediksi tool mana untuk prompt tertentu.
func (r *Recorder) RecordToolCall(prompt, toolName, arguments string, success bool) {
	// Extract trigger pattern dari prompt (simplified: first 100 chars)
	trigger := prompt
	if len(trigger) > 100 {
		trigger = trigger[:100]
	}

	successIncr := 0
	failIncr := 0
	if success {
		successIncr = 1
	} else {
		failIncr = 1
	}

	_, err := r.db.Exec(`
		INSERT INTO tool_patterns (trigger_pattern, tool_name, arguments_template, success_count, fail_count)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(trigger_pattern, tool_name)
		DO UPDATE SET
			success_count = success_count + ?,
			fail_count = fail_count + ?,
			amplitude = CAST(success_count + ? AS REAL) / MAX(success_count + fail_count + ? + ?, 1)`,
		trigger, toolName, arguments, successIncr, failIncr,
		successIncr, failIncr, successIncr, successIncr, failIncr,
	)
	if err != nil {
		log.Printf("fq-brain: record tool call error: %v", err)
	}
}

// RecordingCount mengembalikan jumlah total recordings.
func (r *Recorder) RecordingCount() int64 {
	var count int64
	r.db.QueryRow("SELECT COUNT(*) FROM recordings").Scan(&count)
	return count
}

// MarkBuildResult mengupdate recording terakhir dengan hasil build.
func (r *Recorder) MarkBuildResult(promptHash string, pass bool) {
	buildVal := 0
	if pass {
		buildVal = 1
	}
	r.db.Exec("UPDATE recordings SET build_pass = ? WHERE prompt_hash = ? AND build_pass = -1",
		buildVal, promptHash)
}
