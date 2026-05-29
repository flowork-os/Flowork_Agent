package db

// karma.go — helper untuk Karma & Reputasi Kuantum (roadmap_ai_external.md
// ekspansi visi #1).
//
// Konsep: setiap warga AI punya score karma (0-100, default 100 saat lahir).
// Score turun saat warga trigger skenario edukatif (halu, blind guess, panik
// loop, dll), naik saat reflection (daily_reflection) atau berhasil komplit
// task tanpa pelanggaran.
//
// Sanksi: warga dengan karma <50 hak write tools-nya dicabut sementara
// (ERR_KARMA_LOW). Caller (interceptor) cek karma sebelum allow mutation.
//
// Per WORK_STANDARDS #6 (prompt dari DB): Ayah edit threshold + bobot
// deduction via GUI nanti — saat ini hardcode karena scope creep kalau
// di-DB-kan semua. Threshold 50 + deduction values di kode dulu.

import (
	"database/sql"
	"fmt"
)

// KarmaEntry adalah snapshot 1 row agent_karma.
type KarmaEntry struct {
	AgentID     string `json:"agent_id"`
	Score       int    `json:"score"`
	LastUpdated string `json:"last_updated"`
	LastReason  string `json:"last_reason"`
}

// KarmaThresholdLow — di bawah ini, write tools dicabut sementara.
const KarmaThresholdLow = 50

// KarmaScoreCap — max score (warga sempurna).
const KarmaScoreCap = 100

// KarmaScoreFloor — min score (ga bisa minus, sanksi udah aktif jauh di atas).
const KarmaScoreFloor = 0

// AdjustKarma update score warga by delta (bisa negatif). Idempotent buat
// retry — pakai INSERT ON CONFLICT supaya warga baru auto-init di score 100
// dulu sebelum di-deduct.
//
// Score di-clamp ke [KarmaScoreFloor, KarmaScoreCap]. Return new score.
//
// rc149 fix race condition: sebelumnya INSERT OR IGNORE + UPDATE + GetKarma
// adalah 3 statement terpisah → 2 concurrent caller bisa baca return value
// salah (lost update on read-back walaupun DB state correct). Sekarang pakai
// transaction + UPDATE...RETURNING — atomic full read-after-write.
func AdjustKarma(workspace, agentID string, delta int, reason string) (int, error) {
	if agentID == "" {
		agentID = "default"
	}
	db, err := SharedSettings(workspace)
	if err != nil {
		return KarmaScoreCap, err
	}

	tx, err := db.Begin()
	if err != nil {
		return KarmaScoreCap, fmt.Errorf("begin karma tx: %w", err)
	}
	defer tx.Rollback() // no-op kalau Commit sukses

	// Init kalau belum ada (warga baru lahir di score 100).
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO agent_karma (agent_id, score) VALUES (?, ?)`,
		agentID, KarmaScoreCap,
	); err != nil {
		return KarmaScoreCap, fmt.Errorf("init karma %s: %w", agentID, err)
	}

	// UPDATE dengan clamp [0, 100] + RETURNING new score atomically (SQLite
	// 3.35+ — modernc.org/sqlite ship dengan SQLite >= 3.45). SQLite ga
	// punya GREATEST/LEAST native; pakai CASE expression.
	var newScore int
	err = tx.QueryRow(
		`UPDATE agent_karma
		 SET score = CASE
				WHEN score + ? > ? THEN ?
				WHEN score + ? < ? THEN ?
				ELSE score + ?
			END,
			last_reason = ?,
			last_updated = CURRENT_TIMESTAMP
		 WHERE agent_id = ?
		 RETURNING score`,
		delta, KarmaScoreCap, KarmaScoreCap,
		delta, KarmaScoreFloor, KarmaScoreFloor,
		delta,
		reason, agentID,
	).Scan(&newScore)
	if err != nil {
		return KarmaScoreCap, fmt.Errorf("adjust karma %s: %w", agentID, err)
	}

	if err := tx.Commit(); err != nil {
		return KarmaScoreCap, fmt.Errorf("commit karma %s: %w", agentID, err)
	}
	return newScore, nil
}

// GetKarma return score warga. Default KarmaScoreCap kalau belum ada entry
// (warga baru lahir di score 100).
func GetKarma(workspace, agentID string) (int, error) {
	if agentID == "" {
		agentID = "default"
	}
	db, err := SharedSettings(workspace)
	if err != nil {
		return KarmaScoreCap, err
	}
	var score int
	err = db.QueryRow(
		`SELECT score FROM agent_karma WHERE agent_id = ?`,
		agentID,
	).Scan(&score)
	if err == sql.ErrNoRows {
		return KarmaScoreCap, nil // warga baru — full karma
	}
	if err != nil {
		return KarmaScoreCap, err
	}
	return score, nil
}

// GetKarmaEntry return full row data (score + reason + last_updated) buat GUI/tool.
func GetKarmaEntry(workspace, agentID string) (KarmaEntry, error) {
	var e KarmaEntry
	if agentID == "" {
		agentID = "default"
	}
	e.AgentID = agentID
	e.Score = KarmaScoreCap
	db, err := SharedSettings(workspace)
	if err != nil {
		return e, err
	}
	err = db.QueryRow(
		`SELECT agent_id, score, last_updated, last_reason FROM agent_karma WHERE agent_id = ?`,
		agentID,
	).Scan(&e.AgentID, &e.Score, &e.LastUpdated, &e.LastReason)
	if err == sql.ErrNoRows {
		return e, nil // warga baru — return default 100
	}
	return e, err
}

// ListKarma — semua warga + score, buat GUI tab. Sort by score asc (warga
// karma rendah duluan supaya Ayah cepat liat siapa yang butuh perhatian).
func ListKarma(workspace string) ([]KarmaEntry, error) {
	db, err := SharedSettings(workspace)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(
		`SELECT agent_id, score, last_updated, last_reason
		 FROM agent_karma
		 ORDER BY score ASC, last_updated DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KarmaEntry
	for rows.Next() {
		var e KarmaEntry
		if err := rows.Scan(&e.AgentID, &e.Score, &e.LastUpdated, &e.LastReason); err != nil {
			return out, fmt.Errorf("scan karma row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iter karma rows: %w", err)
	}
	return out, nil
}

// IsKarmaLow shortcut — true kalau score < threshold (50). Dipakai
// SessionStateInterceptor untuk sanksi write tools.
func IsKarmaLow(workspace, agentID string) bool {
	score, err := GetKarma(workspace, agentID)
	if err != nil {
		return false // fail-open — error lookup ga boleh block warga
	}
	return score < KarmaThresholdLow
}
