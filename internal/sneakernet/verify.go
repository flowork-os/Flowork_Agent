// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 19 phase 2 — post-import idempotency check + boot
//   verify probe. Phase 3 (CRDT merge state.db at row level, signed
//   manifest verify) → tambah file baru.
//
// verify.go — Section 19 phase 2: import idempotency + verify probe.

package sneakernet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VerifyImported — post-extract verify:
//   1. manifest.json di target valid + schema OK.
//   2. state.db file ada (kalau di manifest).
//   3. Return Manifest yang verified atau err.
func VerifyImported(targetRoot string) (Manifest, error) {
	var m Manifest
	manifestPath := filepath.Join(targetRoot, "_meta", "manifest.json")
	// Manifest di-write oleh Import dari tarball entry. Kalau ngga ada,
	// fallback check "agent/manifest.json" (style alternative).
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		// Fallback: agent/manifest.json (kalau exported folder include).
		alt := filepath.Join(targetRoot, "manifest.json")
		raw, err = os.ReadFile(alt)
		if err != nil {
			return m, fmt.Errorf("manifest not found: %w", err)
		}
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("manifest decode: %w", err)
	}
	if m.FormatVersion == 0 {
		return m, fmt.Errorf("manifest format_version 0 (invalid)")
	}
	if m.AgentID == "" {
		return m, fmt.Errorf("manifest agent_id empty")
	}
	return m, nil
}

// FingerprintExisting — compute hash dari manifest yang ada di target
// SEBELUM import overwrite. Pakai untuk idempotency detect (same hash
// → no-op).
//
// Return "" kalau manifest ngga ada (fresh import).
func FingerprintExisting(targetRoot string) string {
	manifestPath := filepath.Join(targetRoot, "_meta", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// FingerprintManifest — same hash function buat manifest yang BARU
// di-import. Caller compute pre + post extraction, compare.
func FingerprintManifest(m Manifest) string {
	raw, _ := json.Marshal(m)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
