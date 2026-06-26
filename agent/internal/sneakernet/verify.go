// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package sneakernet

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func VerifyImported(targetRoot string) (Manifest, error) {
	var m Manifest
	manifestPath := filepath.Join(targetRoot, "_meta", "manifest.json")

	raw, err := os.ReadFile(manifestPath)
	if err != nil {

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

func FingerprintExisting(targetRoot string) string {
	manifestPath := filepath.Join(targetRoot, "_meta", "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func FingerprintManifest(m Manifest) string {
	raw, _ := json.Marshal(m)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
