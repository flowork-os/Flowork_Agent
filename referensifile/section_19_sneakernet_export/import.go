// Package sneakernet — kernel/mesh/sneakernet/import.go
//
// M06 USB Sneakernet Import: unpack + verify USB packet, apply ke local
// EventLog (idempotent via HLC-based dedup).
//
// Verify flow (FQP-1 Verify Gate):
//
//	1. Read manifest.json + signature.bin + events.jsonl.
//	2. Verify Ed25519 signature over (FLOWORK-SNEAKERNET-V1\x00 || manifest_canonical || merkle_root).
//	3. Compute Merkle root dari events.jsonl, compare ke manifest.MerkleRoot.
//	4. Verify peer trusted (kalau peer registry ada — optional kalau first
//	   contact, kernel boleh accept dengan low-karma quarantine).
//	5. Apply events ke local log (HLC dedup handle re-import idempotently).

package sneakernet

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flowork/kernel/kernel/mesh"
)

// ImportResult — apa yang ke-apply dari USB packet.
type ImportResult struct {
	Manifest      PacketManifest `json:"manifest"`
	EventsApplied int            `json:"events_applied"`
	EventsTotal   int            `json:"events_total"`
	VerifyDetail  string         `json:"verify_detail"`
}

// Import unpack + verify USB packet, apply events ke local EventLog.
//
// Args:
//   - usbSourceDir: path ke folder USB yang berisi manifest.json + events.jsonl + signature.bin
//
// Return error fatal kalau:
//   - file missing / corrupt
//   - signature invalid
//   - Merkle root mismatch (tamper)
//   - HLC clock skew >10 menit (anti malicious future timestamp)
//
// Apply events idempotent — re-import packet sama = no-op (HLC dedup).
func Import(usbSourceDir string) (*ImportResult, error) {
	// 1. Read manifest.
	manifestPath := filepath.Join(usbSourceDir, "manifest.json")
	mfData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Import: read manifest: %w", err)
	}
	var manifest PacketManifest
	if err := json.Unmarshal(mfData, &manifest); err != nil {
		return nil, fmt.Errorf("sneakernet.Import: parse manifest: %w", err)
	}
	if manifest.Version != "1.0" {
		return nil, fmt.Errorf("sneakernet.Import: unsupported manifest version %q", manifest.Version)
	}

	// 2. Read signature.
	sigPath := filepath.Join(usbSourceDir, "signature.bin")
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Import: read sig: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("sneakernet.Import: invalid sig size %d (want %d)", len(sig), ed25519.SignatureSize)
	}

	// 3. Decode exporter pubkey.
	pubkeyBytes, err := hex.DecodeString(manifest.FromPubKey)
	if err != nil || len(pubkeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("sneakernet.Import: invalid from_pubkey: %v", err)
	}
	pubkey := ed25519.PublicKey(pubkeyBytes)

	// 4. Read events.jsonl + compute Merkle root.
	eventsPath := filepath.Join(usbSourceDir, "events.jsonl")
	ef, err := os.Open(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Import: open events: %w", err)
	}
	defer ef.Close()

	scanner := bufio.NewScanner(ef)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	events := make([]mesh.Event, 0, manifest.EventCount)
	hashes := make([][]byte, 0, manifest.EventCount)
	for scanner.Scan() {
		var ev mesh.Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return nil, fmt.Errorf("sneakernet.Import: parse event: %w", err)
		}
		events = append(events, ev)
		evCanonical, _ := json.Marshal(ev)
		h := sha256.Sum256(evCanonical)
		hashes = append(hashes, h[:])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("sneakernet.Import: scan events: %w", err)
	}
	if len(events) != manifest.EventCount {
		return nil, fmt.Errorf("sneakernet.Import: event count mismatch (manifest=%d actual=%d) — TAMPER", manifest.EventCount, len(events))
	}

	merkleRoot := merkleTreeRoot(hashes)
	expectedRoot, err := hex.DecodeString(manifest.MerkleRoot)
	if err != nil || !bytes.Equal(merkleRoot, expectedRoot) {
		return nil, fmt.Errorf("sneakernet.Import: Merkle root mismatch — TAMPER detected")
	}

	// 5. Verify signature: prefix||manifest||root.
	signPayload := append([]byte("FLOWORK-SNEAKERNET-V1\x00"), mfData...)
	signPayload = append(signPayload, merkleRoot...)
	if !ed25519.Verify(pubkey, signPayload, sig) {
		return nil, fmt.Errorf("sneakernet.Import: Ed25519 signature INVALID — possible forgery")
	}

	// 6. Apply events ke local log (idempotent via HLC dedup).
	log, err := mesh.OpenEventLog()
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Import: open local log: %w", err)
	}
	applied, err := log.Apply(events)
	if err != nil {
		return nil, fmt.Errorf("sneakernet.Import: apply: %w", err)
	}

	return &ImportResult{
		Manifest:      manifest,
		EventsApplied: len(applied),
		EventsTotal:   len(events),
		VerifyDetail:  "ok: signature valid + Merkle root match",
	}, nil
}
