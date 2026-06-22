// cognitive_archive_job.go — D (PHASE 5) host orchestrator: cold-archive node graph tua+
// low-hit per-agent, tiap hari. GATED (activateThreshold) → no-op sampe graph beneran gede,
// jadi 0 dampak di skala sekarang (~2k node). Logika di agentdb.ArchiveColdNodes (reversible).

package main

import (
	"log"

	"flowork-gui/internal/kernelhost"
)

const (
	archiveOlderThanDays     = 90
	archiveMaxHit            = 1
	archiveActivateThreshold = 50000 // cuma archive pas agent punya > segini node aktif (anti-premature)
)

// ArchiveColdNodesAllAgents — sweep cold-archive semua agent. Return total node ke-archive.
func ArchiveColdNodesAllAgents(host *kernelhost.Host) int {
	total := 0
	for _, id := range host.AgentIDs() {
		store, err := host.OpenAgentStore(id)
		if err != nil {
			continue
		}
		n, aerr := store.ArchiveColdNodes(archiveOlderThanDays, archiveMaxHit, archiveActivateThreshold)
		if aerr == nil && n > 0 {
			total += n
			log.Printf("[cold-archive] %s archived %d cold nodes (reversible)", id, n)
		}
		store.Close()
	}
	return total
}
