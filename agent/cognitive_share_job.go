// cognitive_share_job.go — C (PHASE 4) COLLECTIVE GRAPH: promote fakta UMUM (concept/skill/
// knowledge) dari graph LOKAL agent → shared-brain router → agent lain bisa recall pengetahuan
// umum yg udah ditemuin. REUSE SelectPromotableCognitiveNodes/Edges (federation_cognitive.go,
// udah ada gate privasi default-DENY) + PromoteDrawer (jalur teruji) + pola INC-4. Ticker non-
// beku, resilient. Fresh-recall lewat F5 (mem_type 'collective_knowledge' masuk freshMemTypes).
//
// ⚠️ PRIVASI D8 (LANTAI KERAS — concern #1 owner): 3 lapis. (1) Selector default-DENY: cuma
// concept/skill/knowledge + verified + active + BUKAN nyambung identitas owner (person-edge).
// (2) Double-check DETERMINISTIK di sini: kalau strip path/nama/brand NGUBAH konten → SKIP
// (cuma yg 100% bersih yg keluar). (3) Reversible (belum push). "personal GAK ikut" = WAJIB.

package main

import (
	"context"
	"log"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/routerclient"
)

const collectiveMemType = "collective_knowledge"

// cleanForShare — true kalau konten 100% aman keluar agent (strip ga ngubah apa pun + no
// brand). names = allowlist nama owner (runtime). KETAT: ragu = JANGAN share.
func cleanForShare(content string, names []string) bool {
	c := strings.TrimSpace(content)
	if c == "" {
		return false
	}
	return agentdb.StripRecoveryContent(c, names) == c && !agentdb.ContainsBrand(c)
}

// PromoteCognitiveShared — promote node + edge UMUM tiap agent ke shared-brain. Return jumlah
// ke-share. Resilient: router mati → stop agent itu tick ini (retry berikut).
func PromoteCognitiveShared(ctx context.Context, host *kernelhost.Host) int {
	rc := routerclient.New("")
	push := func(content, wing, room string) (string, error) {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		resp, err := rc.PromoteDrawer(pctx, routerclient.PromoteDrawerReq{
			Content: content, Wing: wing, Room: room, MemType: collectiveMemType,
		})
		return resp.ID, err
	}

	shared := 0
	for _, id := range host.AgentIDs() {
		store, err := host.OpenAgentStore(id)
		if err != nil {
			continue
		}
		names := store.OwnerNameAllowlist()
		agentShared := 0

		// --- NODES (concept/skill/knowledge umum) ---
		if nodes, nerr := store.SelectPromotableCognitiveNodes(50); nerr == nil {
			for _, n := range nodes {
				content := strings.TrimSpace(n.Label)
				if n.Why != "" {
					content += " — " + strings.TrimSpace(n.Why)
				}
				if !cleanForShare(content, names) {
					_ = store.MarkPromotedCognitive("node:"+n.ID, "", "blocked-privacy")
					continue
				}
				rid, perr := push(content, "collective", n.Type)
				if perr != nil {
					break // router mati → stop, retry tick berikut
				}
				_ = store.MarkPromotedCognitive("node:"+n.ID, rid, "ok")
				agentShared++
			}
		}

		// --- EDGES (relasi UMUM antar node umum) ---
		if edges, eerr := store.SelectPromotableCognitiveEdges(50); eerr == nil {
			for _, e := range edges {
				content := strings.TrimSpace(e.FromLabel) + " —" + e.RelationType + "→ " + strings.TrimSpace(e.ToLabel)
				refKey := "edge:" + e.FromLabel + "|" + e.RelationType + "|" + e.ToLabel
				if !cleanForShare(content, names) {
					_ = store.MarkPromotedCognitive(refKey, "", "blocked-privacy")
					continue
				}
				rid, perr := push(content, "collective", "relation")
				if perr != nil {
					break
				}
				_ = store.MarkPromotedCognitive(refKey, rid, "ok")
				agentShared++
			}
		}

		if agentShared > 0 {
			shared += agentShared
			log.Printf("[collective-graph] %s share %d fakta umum → shared-brain", id, agentShared)
		}
		store.Close()
	}
	return shared
}
