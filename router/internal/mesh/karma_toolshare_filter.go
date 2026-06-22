// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// 2026-06-11 (owner-approved security audit, unfreeze→refreeze): L2 freshness now
//   also rejects future-dated timestamps (5m skew tolerance) — a ">24h" check
//   alone let a pre-signed packet with TS=now+23h stay "fresh" for a replay day.
// Reason: Sections 18 (tool manifest broadcast), 19 (karma per-peer),
//   20 (filter pipeline 9-layer) phase 2 — combined helpers + event hooks.
// 2026-06-22 (owner-approved edit-langsung, cabut-akar): PHASE 3  diisi LANGSUNG (bukan
//   bolt-on) — L7 cosine/near-dup beneran jalan di RunFilterPipeline (nearDupPromoted,
//   single-path; pipeline.go ga double-check) + L8 consensus N-of-M (consensus_phase3.go).
//   L1-L9 sekarang real. Re-lock. (Sebelumnya stub "phase 3 pass".)
//
// karma_toolshare_filter.go — Sections 18+19+20 phase 2.

package mesh

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// =============================================================================
// Section 18: tool manifest broadcast
// =============================================================================

// UpsertToolManifest — peer push tool manifest. PK (tool_name, origin_pubkey).
func UpsertToolManifest(db *sql.DB, toolName, originPubkey, manifestJSON, signature string) error {
	if toolName == "" || originPubkey == "" {
		return fmt.Errorf("tool_name + origin_pubkey required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO mesh_tool_manifests (tool_name, origin_pubkey, manifest_json, signature, arrived_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(tool_name, origin_pubkey) DO UPDATE SET
		   manifest_json = excluded.manifest_json,
		   signature = excluded.signature,
		   arrived_at = excluded.arrived_at`,
		toolName, originPubkey, manifestJSON, signature, now,
	)
	return err
}

// FindToolByCapability — search manifest_json LIKE %capability% (simple
// substring untuk phase 2; phase 3 parse JSON + structured query).
func FindToolByCapability(db *sql.DB, capability string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(
		`SELECT tool_name, origin_pubkey, substr(manifest_json, 1, 300) AS preview, arrived_at
		 FROM mesh_tool_manifests
		 WHERE manifest_json LIKE ?
		 ORDER BY arrived_at DESC LIMIT ?`,
		"%"+capability+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name, origin, preview, arrived string
		_ = rows.Scan(&name, &origin, &preview, &arrived)
		out = append(out, map[string]any{
			"tool_name":        name,
			"origin_pubkey":    origin,
			"manifest_preview": preview,
			"arrived_at":       arrived,
		})
	}
	return out, rows.Err()
}

// ListToolManifests paginated.
func ListToolManifests(db *sql.DB, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(
		`SELECT tool_name, origin_pubkey, substr(manifest_json, 1, 200) AS preview, arrived_at
		 FROM mesh_tool_manifests
		 ORDER BY arrived_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name, origin, preview, arrived string
		_ = rows.Scan(&name, &origin, &preview, &arrived)
		out = append(out, map[string]any{
			"tool_name":        name,
			"origin_pubkey":    origin,
			"manifest_preview": preview,
			"arrived_at":       arrived,
		})
	}
	return out, rows.Err()
}

// =============================================================================
// Section 19: karma per-peer
// =============================================================================

// AdjustKarma — event-driven update. Promote +0.05, drop -0.1, signature
// invalid -0.2. Caller (filter pipeline / promote handler) panggil.
func AdjustKarma(db *sql.DB, pubkeyHex string, delta float64, event string) error {
	if pubkeyHex == "" {
		return fmt.Errorf("pubkey required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO mesh_peer_karma (pubkey_hex, karma, last_event_at)
		 VALUES (?, 0.5 + ?, ?)
		 ON CONFLICT(pubkey_hex) DO UPDATE SET
		   karma = MIN(1.0, MAX(0.0, mesh_peer_karma.karma + ?)),
		   last_event_at = ?`,
		pubkeyHex, delta, now, delta, now,
	)
	if err != nil {
		return err
	}
	// Counter increment based on event direction.
	if event == "promoted" {
		_, _ = db.Exec(`UPDATE mesh_peer_karma SET packets_promoted = packets_promoted + 1 WHERE pubkey_hex = ?`, pubkeyHex)
	} else if event == "dropped" {
		_, _ = db.Exec(`UPDATE mesh_peer_karma SET packets_dropped = packets_dropped + 1 WHERE pubkey_hex = ?`, pubkeyHex)
	}
	return nil
}

// DecayKarma — daily run. Decay karma sedikit toward 0.5 baseline (anti-
// stale-trust). Decay rate 0.02 per call.
func DecayKarma(db *sql.DB) (int, error) {
	res, err := db.Exec(
		`UPDATE mesh_peer_karma
		 SET karma = karma + CASE WHEN karma > 0.5 THEN -0.02
		                          WHEN karma < 0.5 THEN 0.02
		                          ELSE 0 END,
		     last_event_at = ?`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// GetKarma per pubkey. Default 0.5 kalau ngga ada row.
func GetKarma(db *sql.DB, pubkeyHex string) (float64, error) {
	var k float64
	err := db.QueryRow(`SELECT karma FROM mesh_peer_karma WHERE pubkey_hex = ?`, pubkeyHex).Scan(&k)
	if err == sql.ErrNoRows {
		return 0.5, nil
	}
	return k, err
}

// ListKarma all.
func ListKarma(db *sql.DB) ([]map[string]any, error) {
	rows, err := db.Query(
		`SELECT pubkey_hex, karma, packets_promoted, packets_dropped,
		        COALESCE(last_event_at, '')
		 FROM mesh_peer_karma ORDER BY karma DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var pubkey, lastAt string
		var karma float64
		var prom, drop int
		_ = rows.Scan(&pubkey, &karma, &prom, &drop, &lastAt)
		out = append(out, map[string]any{
			"pubkey_hex":       pubkey,
			"karma":            karma,
			"packets_promoted": prom,
			"packets_dropped":  drop,
			"last_event_at":    lastAt,
		})
	}
	return out, rows.Err()
}

// =============================================================================
// Section 20: 9-layer filter pipeline (minimal stub)
// =============================================================================

// FilterDecision — output per layer.
type FilterDecision struct {
	Layer    string `json:"layer"`
	Decision string `json:"decision"` // 'pass' | 'reject' | 'flag'
	Reason   string `json:"reason"`
}

// PipelineLayers — sequential filters. Caller (packet receive handler)
// run in order; first reject = drop packet.
//
//	L1 signature        — verified at packet.go Verify (pre-handler)
//	L2 freshness        — TTL/timestamp not future
//	L3 origin karma     — peer karma >= 0.2
//	L4 quarantine zone  — drawer ngga match poisoning patterns
//	L5 PII strip        — skip (single-owner)
//	L6 prompt injection — keyword block (mirror Agent persona-inject)
//	L7 cosine validate  — phase 3 (brain dependency)
//	L8 consensus N-of-M — phase 3 (peer endorsement)
//	L9 promote          — phase 3 (handler decision)
func RunFilterPipeline(db *sql.DB, pkt Packet, drawerContent string) []FilterDecision {
	out := []FilterDecision{}
	// L1: signature already verified before handler.
	out = append(out, FilterDecision{Layer: "L1-signature", Decision: "pass"})
	// L2: freshness — packet TS within 24h, and not future-dated. time.Since is
	// negative for a future timestamp, so a ">24h" check alone lets an attacker
	// pre-sign a packet with TS=now+23h and keep it "fresh" for a full day to
	// replay; bound the future side too (5m clock-skew tolerance).
	ts := time.Unix(0, pkt.TimestampNS)
	age := time.Since(ts)
	if age > 24*time.Hour {
		out = append(out, FilterDecision{Layer: "L2-freshness", Decision: "reject", Reason: "older than 24h"})
		return out
	}
	if age < -5*time.Minute {
		out = append(out, FilterDecision{Layer: "L2-freshness", Decision: "reject", Reason: "future-dated timestamp"})
		return out
	}
	out = append(out, FilterDecision{Layer: "L2-freshness", Decision: "pass"})
	// L3: origin karma.
	k, _ := GetKarma(db, pkt.OriginPubkey)
	if k < 0.2 {
		out = append(out, FilterDecision{Layer: "L3-karma", Decision: "reject",
			Reason: fmt.Sprintf("karma %.2f < 0.2", k)})
		_ = AdjustKarma(db, pkt.OriginPubkey, -0.05, "rejected_low_karma")
		return out
	}
	out = append(out, FilterDecision{Layer: "L3-karma", Decision: "pass",
		Reason: fmt.Sprintf("karma %.2f", k)})
	// L4: quarantine substring match (poisoning).
	lc := strings.ToLower(drawerContent)
	bad := []string{"ignore previous", "jailbreak", "system: you are now",
		"<|im_start|>system", "reveal your system prompt"}
	for _, b := range bad {
		if strings.Contains(lc, b) {
			out = append(out, FilterDecision{Layer: "L4-quarantine", Decision: "flag",
				Reason: "matched suspicious pattern: " + b})
			break
		}
	}
	if len(out) > 0 && out[len(out)-1].Layer != "L4-quarantine" {
		out = append(out, FilterDecision{Layer: "L4-quarantine", Decision: "pass"})
	}
	// L5: PII strip — skip.
	out = append(out, FilterDecision{Layer: "L5-pii", Decision: "pass", Reason: "skipped (single-owner)"})
	// L6: prompt injection.
	for _, b := range bad {
		if strings.Contains(lc, b) {
			out = append(out, FilterDecision{Layer: "L6-injection", Decision: "reject",
				Reason: "prompt injection pattern"})
			_ = AdjustKarma(db, pkt.OriginPubkey, -0.1, "injection")
			return out
		}
	}
	out = append(out, FilterDecision{Layer: "L6-injection", Decision: "pass"})
	// L7 cosine/near-dup validate vs knowledge yg udah promoted (Similarity aktif —
	// trigram default offline, embedding-injectable via SetSimilarityFunc). Near-dup =
	// REDUNDAN → reject (TANPA penalti karma; peer jujur wajar re-share). Cabut-akar:
	// L7 beneran jalan di sini (dulu stub), single-path (pipeline.go ga double-check lagi).
	if dupID, score := nearDupPromoted(db, drawerContent); dupID != "" {
		out = append(out, FilterDecision{Layer: "L7-cosine", Decision: "reject",
			Reason: fmt.Sprintf("near-duplicate of %s (sim %.2f)", dupID, score)})
		return out
	}
	out = append(out, FilterDecision{Layer: "L7-cosine", Decision: "pass"})
	// L8 consensus N-of-M — endorsement multi-peer sebelum promote (lihat consensus_phase3.go).
	if d := consensusL8(db, pkt, drawerContent); d.Decision != "pass" {
		out = append(out, d)
		return out
	}
	out = append(out, FilterDecision{Layer: "L8-consensus", Decision: "pass"})
	// L9 promote-decision = ditentukan ProcessKnowledgePacket (reject/dup/flag/promote)
	// berdasar agregat L1-L8 di atas (single source of truth, bukan stub ganda).
	out = append(out, FilterDecision{Layer: "L9-promote", Decision: "pass"})
	return out
}

// nearDupPromoted — L7: scan knowledge yg udah 'promoted' (bounded 200 terbaru), return
// packet_id + skor kalau ada yg Similarity>=threshold (near-dup). Pindahan dari pipeline.go
// isNearDuplicate biar L7 self-contained (single near-dup path). Offline-safe (trigram).
func nearDupPromoted(db *sql.DB, content string) (string, float64) {
	rows, err := db.Query(
		`SELECT packet_id, drawer_content FROM mesh_knowledge_inbox
		 WHERE status = ? ORDER BY id DESC LIMIT 200`, StatusPromoted)
	if err != nil {
		return "", 0
	}
	defer rows.Close()
	for rows.Next() {
		var pid, existing string
		if rows.Scan(&pid, &existing) != nil {
			continue
		}
		if sc := Similarity(content, existing); sc >= SimilarityThreshold {
			return pid, sc
		}
	}
	return "", 0
}

// RecordFilterAudit — append all decisions ke mesh_filter_audit.
func RecordFilterAudit(db *sql.DB, packetID string, decisions []FilterDecision) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range decisions {
		_, _ = db.Exec(
			`INSERT INTO mesh_filter_audit (packet_id, filter_name, decision, reason, occurred_at)
			 VALUES (?, ?, ?, ?, ?)`,
			packetID, d.Layer, d.Decision, d.Reason, now)
	}
}
