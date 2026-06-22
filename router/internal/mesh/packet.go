// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-os/flow_router
// Locked at: 2026-05-30
// 2026-06-13 (owner-approved, autonomous mesh task): persist + reload the signed TimestampNS
//   (PersistPacket / ListPendingPackets) so a gossiped packet keeps the exact canonical bytes it
//   was signed with — otherwise every forwarded packet failed ed25519 verify on the receiver and
//   the mesh never delivered anything. Bug fix only; signing/verify logic unchanged.
// Reason: Section 14 phase 2 packet structure + canonical signing. ed25519
//   sign(sha256(canonical bytes)). Phase 3 (CBOR encoding, deterministic
//   map iteration for nested objects) → tambah file baru.
//
// packet.go — Section 14 phase 2: KnowledgePacket + canonical sign/verify.

package mesh

import (
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PacketType — enum.
const (
	PacketTypeKnowledge   = "knowledge"
	PacketTypeTask        = "task"
	PacketTypeGossip      = "gossip"
	PacketTypeHeartbeat   = "heartbeat"
	PacketTypeToolShare   = "tool_share"
	PacketTypeMistakePush = "mistake_push"
	PacketTypeSunsetNotice = "sunset_notice"
	PacketTypeEmergency   = "emergency_alert"
)

// HopMax — anti-flood. Per FQP doctrine.
const HopMax = 7

// Packet — unit data exchange antar peer.
type Packet struct {
	PacketID     string `json:"packet_id"`     // UUID-like unique
	OriginPubkey string `json:"origin_pubkey"` // ed25519 pubkey hex
	PacketType   string `json:"packet_type"`
	PayloadJSON  string `json:"payload_json"`  // serialized payload
	Signature    string `json:"signature"`     // ed25519 hex
	TTL          int    `json:"ttl"`           // default 5
	HopCount     int    `json:"hop_count"`
	TimestampNS  int64  `json:"timestamp_ns"`
}

// CanonicalBytes — deterministic byte encoding for signing.
// Format: type || 0x00 || payload || 0x00 || ts_be || 0x00 || packet_id
func (p Packet) CanonicalBytes() []byte {
	buf := []byte{}
	buf = append(buf, []byte(p.PacketType)...)
	buf = append(buf, 0x00)
	buf = append(buf, []byte(p.PayloadJSON)...)
	buf = append(buf, 0x00)
	tsBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(tsBuf, uint64(p.TimestampNS))
	buf = append(buf, tsBuf...)
	buf = append(buf, 0x00)
	buf = append(buf, []byte(p.PacketID)...)
	return buf
}

// Sign — produce ed25519 signature. Caller (origin router) holds privkey.
// Mutates p.Signature in place.
func (p *Packet) Sign(privKey []byte) error {
	if len(privKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid privkey length %d", len(privKey))
	}
	sum := sha256.Sum256(p.CanonicalBytes())
	sig := ed25519.Sign(privKey, sum[:])
	p.Signature = hex.EncodeToString(sig)
	return nil
}

// Verify — return nil kalau signature valid.
func (p Packet) Verify() error {
	pubBytes, err := hex.DecodeString(p.OriginPubkey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid origin_pubkey")
	}
	sigBytes, err := hex.DecodeString(p.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length")
	}
	sum := sha256.Sum256(p.CanonicalBytes())
	if !ed25519.Verify(pubBytes, sum[:], sigBytes) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// NewPacket — factory dengan auto-stamp packet_id (sha256 trunc) + ts.
func NewPacket(originPubkeyHex, packetType, payloadJSON string) Packet {
	ts := time.Now().UTC().UnixNano()
	idSeed := fmt.Sprintf("%s-%s-%d", originPubkeyHex, packetType, ts)
	idSum := sha256.Sum256([]byte(idSeed))
	return Packet{
		PacketID:     hex.EncodeToString(idSum[:])[:16],
		OriginPubkey: originPubkeyHex,
		PacketType:   packetType,
		PayloadJSON:  payloadJSON,
		TTL:          5,
		HopCount:     0,
		TimestampNS:  ts,
	}
}

// =============================================================================
// Persistence
// =============================================================================

// PersistPacket — INSERT into mesh_packets. Skip kalau packet_id exist
// (idempotent — dedup by ID).
func PersistPacket(db *sql.DB, p Packet) error {
	if strings.TrimSpace(p.PacketID) == "" {
		return fmt.Errorf("packet_id required")
	}
	_, err := db.Exec(
		`INSERT OR IGNORE INTO mesh_packets
		   (packet_id, origin_pubkey, packet_type, payload_json, signature,
		    ttl, hop_count, timestamp_ns, received_at, processed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		p.PacketID, p.OriginPubkey, p.PacketType, p.PayloadJSON, p.Signature,
		p.TTL, p.HopCount, p.TimestampNS, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// HasPacket — check kalau packet_id sudah ada (dedupe pre-gossip).
func HasPacket(db *sql.DB, packetID string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM mesh_packets WHERE packet_id = ?`, packetID).Scan(&n)
	return n > 0, err
}

// MarkProcessed — set processed=1 after consumer handled.
func MarkProcessed(db *sql.DB, packetID string) error {
	_, err := db.Exec(`UPDATE mesh_packets SET processed = 1 WHERE packet_id = ?`, packetID)
	return err
}

// ListPendingPackets — return unprocessed packets ordered by received_at.
// Caller (consumer) pakai untuk catch-up after restart.
func ListPendingPackets(db *sql.DB, limit int) ([]Packet, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(
		`SELECT packet_id, origin_pubkey, packet_type, payload_json,
		        signature, ttl, hop_count, timestamp_ns
		 FROM mesh_packets WHERE processed = 0
		 ORDER BY received_at LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Packet{}
	for rows.Next() {
		var p Packet
		if serr := rows.Scan(&p.PacketID, &p.OriginPubkey, &p.PacketType,
			&p.PayloadJSON, &p.Signature, &p.TTL, &p.HopCount, &p.TimestampNS); serr != nil {
			return nil, serr
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// =============================================================================
// Privkey helpers (mesh_identity has privkey_hex stored)
// =============================================================================

// LoadPrivKey — read ed25519 privkey from mesh_identity (kv lookup).
func LoadPrivKey(db *sql.DB) ([]byte, error) {
	var hexStr string
	err := db.QueryRow(`SELECT v FROM mesh_identity WHERE k = 'privkey_hex'`).Scan(&hexStr)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(hexStr)
}

// LoadPubKeyHex — own pubkey hex from mesh_identity.
func LoadPubKeyHex(db *sql.DB) (string, error) {
	var hexStr string
	err := db.QueryRow(`SELECT v FROM mesh_identity WHERE k = 'pubkey_hex'`).Scan(&hexStr)
	return hexStr, err
}

// ParsePacketJSON — helper deserialize untuk HTTP handler.
func ParsePacketJSON(data []byte) (Packet, error) {
	var p Packet
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	return p, nil
}
