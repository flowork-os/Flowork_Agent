// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package sneakernet

import "time"

const (
	FormatVersion = 1

	ManifestPath = "_meta/manifest.json"
)

type Manifest struct {
	FormatVersion int    `json:"format_version"`
	AgentID       string `json:"agent_id"`
	Version       string `json:"version"`
	HostOrigin    string `json:"host_origin"`
	CreatedAt     string `json:"created_at"`
	Encrypted     bool   `json:"encrypted"`
	StateDBBytes  int64  `json:"state_db_bytes"`
	FilesCount    int    `json:"files_count"`
}

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
