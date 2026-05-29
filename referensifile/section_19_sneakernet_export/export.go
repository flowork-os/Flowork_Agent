// Package sneakernet — kernel/mesh/sneakernet/export.go
//
// M06 USB Sneakernet Fallback (ROADMAP_AKTIF.md Tier 4).
//
// Logic: kalau semua transport mati (no internet/LAN/Bluetooth/LoRa),
// pertukaran knowledge antar laptop user via USB stick. Format: signed
// JSONL paket dengan Merkle proof root signed Ed25519.
//
// Pack flow:
//
//	1. Read events sejak HLC tertentu dari EventLog (mesh.HLC).
//	2. Compute Merkle root dari sequence event hash (anti tampering
//	   selektif — drop atau swap entry break Merkle proof).
//	3. Sign Merkle root + manifest pakai kernel Ed25519 identity.
//	4. Write 3 file ke USB target dir:
//	     - manifest.json    (metadata: from_pubkey, since_hlc, count, root_hash)
//	     - events.jsonl     (event payload)
//	     - signature.bin    (Ed25519 sig of manifest+root_hash)
//
// FQP compliance:
//   - FQP-2 Bridge: append-only events JSONL (no edit history)
//   - FQP-12 Append-Only: Merkle proof guarantees full integrity
//   - FQP-1 Verify Gate: signature wajib (import side)
//   - FQP-5 No Wormhole: receiver verify before apply

package sneakernet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flowork/kernel/kernel/identity"
	"github.com/flowork/kernel/kernel/mesh"
)

// PacketManifest — metadata header untuk USB packet.
type PacketManifest struct {
	Version    string    `json:"version"`     // schema version (e.g. "1.0")
	FromPubKey string    `json:"from_pubkey"` // hex Ed25519 pubkey of exporter
	SinceHLC   mesh.HLC  `json:"since_hlc"`   // export from this HLC onwards
	UntilHLC   mesh.HLC  `json:"until_hlc"`   // last event HLC included
	EventCount int       `json:"event_count"`
	MerkleRoot string    `json:"merkle_root"` // hex SHA-256 Merkle root
	CreatedAt  time.Time `json:"created_at"`
	NodeID     string    `json:"node_id"` // exporter NodeID untuk audit
}

// ExportResult — apa yang di-write ke USB.
type ExportResult struct {
	OutputDir   string         `json:"output_dir"`
	Manifest    PacketManifest `json:"manifest"`
	BytesWriten int64          `json:"bytes_written"`
	SigPath     string         `json:"sig_path"`
}

// Export pack events sejak `since` HLC ke USB target dir.
//
// Args:
//   - usbTargetDir: path ke USB stick subfolder (caller siapin: mount + mkdir)
//   - since: export events dengan HLC > since
//
// Sukses → 3 file di usbTargetDir: manifest.json, events.jsonl, signature.bin.
func Export(usbTargetDir string, since mesh.HLC) (*ExportResult, error) {
	if err := os.MkdirAll(usbTargetDir, 0o755); err != nil {
		return nil, fmt.Errorf("sneakernet.Export: mkdir target: %w", err)
	}

	// 1. Open EventLog + read events.
	log, err := mesh.OpenEventLog()
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Export: open event log: %w", err)
	}
	events, err := log.ReadSince(since)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Export: read events: %w", err)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("sneakernet.Export: no events to export sejak HLC %+v", since)
	}

	// 2. Write events.jsonl.
	eventsPath := filepath.Join(usbTargetDir, "events.jsonl")
	ef, err := os.Create(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Export: create events.jsonl: %w", err)
	}
	enc := json.NewEncoder(ef)
	hashes := make([][]byte, 0, len(events))
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			ef.Close()
			return nil, fmt.Errorf("sneakernet.Export: encode event: %w", err)
		}
		// Hash canonical JSON event untuk Merkle leaf.
		evCanonical, _ := json.Marshal(ev)
		h := sha256.Sum256(evCanonical)
		hashes = append(hashes, h[:])
	}
	ef.Close()

	stat, _ := os.Stat(eventsPath)

	// 3. Compute Merkle root.
	merkleRoot := merkleTreeRoot(hashes)

	// 4. Build manifest. identity.Ensure return (peerID, pubkey, err).
	peerID, pub, err := identity.Ensure()
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Export: identity: %w", err)
	}
	_ = peerID // peerID format `flowork-peer://...` — exporter audit trail
	pubkeyHex := hex.EncodeToString(pub)
	manifest := PacketManifest{
		Version:    "1.0",
		FromPubKey: pubkeyHex,
		SinceHLC:   since,
		UntilHLC:   events[len(events)-1].HLC,
		EventCount: len(events),
		MerkleRoot: hex.EncodeToString(merkleRoot),
		CreatedAt:  time.Now().UTC(),
		NodeID:     events[len(events)-1].HLC.NodeID,
	}

	// 5. Write manifest.json.
	manifestPath := filepath.Join(usbTargetDir, "manifest.json")
	mfData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Export: marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, mfData, 0o644); err != nil {
		return nil, fmt.Errorf("sneakernet.Export: write manifest: %w", err)
	}

	// 6. Sign manifest+merkle root via kernel Ed25519 identity.
	// Domain separation: prefix "FLOWORK-SNEAKERNET-V1\x00" (anti
	// cross-protocol forgery — sig dari sneakernet ngga ke-verify
	// di mesh sync verifier kalau prefix beda).
	signPayload := append([]byte("FLOWORK-SNEAKERNET-V1\x00"), mfData...)
	signPayload = append(signPayload, merkleRoot...)
	sig, err := identity.Sign(signPayload)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Export: sign: %w", err)
	}
	sigPath := filepath.Join(usbTargetDir, "signature.bin")
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		return nil, fmt.Errorf("sneakernet.Export: write sig: %w", err)
	}

	return &ExportResult{
		OutputDir:   usbTargetDir,
		Manifest:    manifest,
		BytesWriten: stat.Size(),
		SigPath:     sigPath,
	}, nil
}

// merkleTreeRoot compute SHA-256 Merkle root dari leaf hashes.
// Pair-wise hash, ganjil node duplicate ke kanan (CometBFT-style).
// Empty input → 32-byte zero hash.
func merkleTreeRoot(leaves [][]byte) []byte {
	if len(leaves) == 0 {
		return make([]byte, 32)
	}
	if len(leaves) == 1 {
		return leaves[0]
	}
	level := leaves
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			var pair []byte
			if i+1 < len(level) {
				pair = append(pair, level[i]...)
				pair = append(pair, level[i+1]...)
			} else {
				// Odd: duplicate last leaf.
				pair = append(pair, level[i]...)
				pair = append(pair, level[i]...)
			}
			h := sha256.Sum256(pair)
			next = append(next, h[:])
		}
		level = next
	}
	return level[0]
}
