// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// toolvalidate.go — Section 18 phase 3: tool manifest validation + intake.
//
// Closes the audit gap "manifest validator vs scanner". A shared tool manifest
// from a peer is untrusted code-description: before it lands in
// mesh_tool_manifests (and becomes discoverable via find-tool), it must be
// well-formed and free of obvious danger signals. This is the mesh-side mirror
// of the local MCP allowlist — defense in depth for the cross-host tool market.

package mesh

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ToolShareResult — outcome of ingesting a tool_share packet.
type ToolShareResult struct {
	ToolName string `json:"tool_name"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
}

// dangerousManifestTokens — substrings that have no business in a declarative
// tool manifest and strongly indicate an attempt to smuggle execution.
var dangerousManifestTokens = []string{
	"rm -rf", "sudo ", "curl ", "wget ", "; rm ", "&&rm", "| sh", "|sh",
	"/bin/sh", "/bin/bash", "powershell", "cmd.exe", "os.system(",
	"subprocess", "eval(", "exec(", "__import__", "base64 -d",
	"\\x00", "<script", "data:text/html",
}

// ValidateToolManifest checks a manifest JSON string. Rules:
//   - must parse as a JSON object
//   - must declare a non-empty "name"
//   - must be ≤ 32 KB (anti-bloat / anti-DoS)
//   - must not contain any dangerous execution token (case-insensitive)
//
// Returns (true, "") when acceptable, else (false, reason).
func ValidateToolManifest(manifestJSON string) (bool, string) {
	if len(manifestJSON) == 0 {
		return false, "empty manifest"
	}
	if len(manifestJSON) > 32*1024 {
		return false, "manifest too large (>32KB)"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(manifestJSON), &obj); err != nil {
		return false, "manifest is not a JSON object: " + err.Error()
	}
	if name, _ := obj["name"].(string); strings.TrimSpace(name) == "" {
		return false, "manifest missing \"name\""
	}
	lc := strings.ToLower(manifestJSON)
	for _, tok := range dangerousManifestTokens {
		if strings.Contains(lc, strings.ToLower(tok)) {
			return false, fmt.Sprintf("manifest contains dangerous token %q", tok)
		}
	}
	return true, ""
}

// toolSharePayload — wire shape of a tool_share packet payload.
type toolSharePayload struct {
	ToolName     string `json:"tool_name"`
	ManifestJSON string `json:"manifest_json"`
	Signature    string `json:"signature"`
}

// IngestToolSharePacket parses, validates, and (if clean) stores a peer's tool
// manifest. The packet signature was already verified by the receive handler;
// this adds content validation on top. A rejected manifest costs the origin a
// small karma penalty so repeat offenders gate out.
func IngestToolSharePacket(db *sql.DB, pkt Packet) ToolShareResult {
	var p toolSharePayload
	if err := json.Unmarshal([]byte(pkt.PayloadJSON), &p); err != nil {
		return ToolShareResult{Accepted: false, Reason: "bad payload: " + err.Error()}
	}
	if strings.TrimSpace(p.ToolName) == "" {
		return ToolShareResult{Accepted: false, Reason: "tool_name required"}
	}
	if ok, reason := ValidateToolManifest(p.ManifestJSON); !ok {
		_ = AdjustKarma(db, pkt.OriginPubkey, -0.1, "bad_tool_manifest")
		return ToolShareResult{ToolName: p.ToolName, Accepted: false, Reason: reason}
	}
	if err := UpsertToolManifest(db, p.ToolName, pkt.OriginPubkey, p.ManifestJSON, p.Signature); err != nil {
		return ToolShareResult{ToolName: p.ToolName, Accepted: false, Reason: err.Error()}
	}
	return ToolShareResult{ToolName: p.ToolName, Accepted: true, Reason: "validated + stored"}
}
