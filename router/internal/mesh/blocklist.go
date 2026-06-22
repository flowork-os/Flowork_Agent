// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 13 phase 1 (INVARIANT 2 cloud metadata blocklist).
//   API stable: IsCloudMetadataIP / IsCloudMetadataHost. Phase 2 extend
//   denylist → tambah file baru, JANGAN modify ini.
//
// blocklist.go — cloud metadata IP denylist (INVARIANT 2).
//
// Per ANTI_KIAMAT_PROTOCOL.md §INVARIANT 2: mDNS / peer discovery DILARANG
// terima route ke cloud metadata IP (169.254.169.254 dst.). Tanpa ini,
// SSRF + pivot attack via mesh discovery jadi mungkin.

package mesh

import "net"

// cloudMetadataIPs — known cloud provider metadata service IPs.
var cloudMetadataIPs = map[string]bool{
	"169.254.169.254": true, // AWS, GCP, Azure
	"100.100.100.200": true, // Alibaba Cloud
	"192.0.0.192":     true, // legacy / reserved
}

// cloudMetadataDNS — known metadata hostnames (DNS spoofing defense).
var cloudMetadataDNS = map[string]bool{
	"metadata.google.internal":   true,
	"instance-data.ec2.internal": true,
	"metadata.azure.com":         true,
	"metadata.internal":          true,
}

// linkLocalCIDR — entire 169.254.0.0/16 range (paranoid block).
var linkLocalCIDR *net.IPNet

func init() {
	_, linkLocalCIDR, _ = net.ParseCIDR("169.254.0.0/16")
}

// IsCloudMetadataIP returns true if IP matches known cloud metadata
// endpoints or falls within the link-local range.
func IsCloudMetadataIP(ip string) bool {
	if cloudMetadataIPs[ip] {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if linkLocalCIDR != nil && linkLocalCIDR.Contains(parsed) {
		return true
	}
	// IPv6 link-local (fe80::/10) + multicast also reach link-local services /
	// IPv6 metadata; IsLinkLocalUnicast covers v4 169.254 too (defence in depth).
	if parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() {
		return true
	}
	return false
}

// IsCloudMetadataHost returns true if hostname matches known cloud metadata DNS.
func IsCloudMetadataHost(host string) bool {
	return cloudMetadataDNS[host]
}
