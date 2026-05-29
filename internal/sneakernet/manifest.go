// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 19 phase 1 — sneakernet manifest format. JSON shape
//   buat header tarball: agent_id, version, host_origin, contents stat.
//   Phase 2 (signed_origin, mesh_peers_cache, CRDT merge token) → tambah
//   field baru, JANGAN modify ini.
//
// manifest.go — Section 19 phase 1: sneakernet manifest.

package sneakernet

import "time"

const (
	// FormatVersion — bump kalau breaking schema.
	FormatVersion = 1
	// ManifestPath — entry path di tarball.
	ManifestPath = "_meta/manifest.json"
)

// Manifest — header pertama di .fwsync tarball.
type Manifest struct {
	FormatVersion int    `json:"format_version"`
	AgentID       string `json:"agent_id"`
	Version       string `json:"version"`        // agent manifest version
	HostOrigin    string `json:"host_origin"`    // hostname asal export
	CreatedAt     string `json:"created_at"`     // RFC3339 UTC
	Encrypted     bool   `json:"encrypted"`      // true → AES-256-GCM
	StateDBBytes  int64  `json:"state_db_bytes"` // size state.db (snapshot via VACUUM INTO)
	FilesCount    int    `json:"files_count"`    // count file di tarball (selain manifest)
}

// NewManifest — preset format_version + created_at.
func NewManifest(agentID, version, hostOrigin string, encrypted bool) Manifest {
	return Manifest{
		FormatVersion: FormatVersion,
		AgentID:       agentID,
		Version:       version,
		HostOrigin:    hostOrigin,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Encrypted:     encrypted,
	}
}
