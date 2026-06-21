// mistake_promote_job.go — D32 inc-1: promote mistake RECURRING → recovery-instinct
// (embedded, semantic-recallable). Non-beku (package main = wiring), mirror pola
// wakeup_engine/task_worker.
//
// AKAR (D32): Flowork UDAH punya loop mistakes (AddMistake hit_count + tier
// raw/reviewed/promoted + ListMistakesEligibleForPromote gate). GAP: (1) gate-nya GA
// di-wire ke ticker (mistake numpuk di 'raw', ga pernah promote); (2) recall mistakes =
// LIKE (keyword), BUKAN semantik. Job ini tutup dua-duanya: mistake yg hit_count>=N
// (REPETISI = sinyal kualitas, anti-degenerasi SGS) → project jadi cognitive_node
// type='instinct' where_domain='recovery' + embedding → ke-recall by-MAKNA via
// instinct_recall pas error MIRIP muncul. SetMistakePromoted = anti re-sweep.
//
// REUSE (cabut-akar, jangan numpuk): ListMistakesEligibleForPromote (gate, file
// LOCKED-nya GA disentuh) · UpsertNode+embedding (pola instproj) · EmbedText/Quantize.
// Inc-1 = projeksi DETERMINISTIK (no LLM, cheap+testable). Generalisasi via dream-
// digester + auto-capture error→recovery + share-ke-router-antibody = inc berikut.
package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/routerclient"
)

// promoteMinHit — REPETISI minimum sebelum mistake jadi recovery-instinct. >=3 =
// konservatif (cuma pola yg BENERAN keulang; sekali-error ga langsung jadi insting →
// anti-degenerasi). LOCKED soft: jangan turunin di bawah 2 tanpa alasan.
const promoteMinHit = 3

// promoteBrandRe — leak-gate white-label (sama doktrin instinct seed). Brand → skip.
var promoteBrandRe = regexp.MustCompile(`(?i)\b(claude|anthropic|fable|gemini|opus|sonnet|haiku|chatgpt|openai)\b`)

// PromoteRecurringMistakes — sweep semua agent: mistake recurring → recovery-instinct
// embedded. Return jumlah yang di-promote tick ini. Idempotent (SetMistakePromoted
// nge-exclude dari sweep berikut).
func PromoteRecurringMistakes(ctx context.Context, host *kernelhost.Host) int {
	rc := routerclient.New("")
	promoted := 0
	for _, id := range host.AgentIDs() {
		store, err := host.OpenAgentStore(id)
		if err != nil {
			continue
		}
		// Skip murah kalau ga ada tabel mistakes_local (agent ga pernah catat mistake).
		var tbl string
		if store.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='mistakes_local'").Scan(&tbl) != nil {
			store.Close()
			continue
		}
		eligible, lerr := store.ListMistakesEligibleForPromote(promoteMinHit, 50)
		if lerr != nil || len(eligible) == 0 {
			store.Close()
			continue
		}
		scope := "agent:" + id
		for _, m := range eligible {
			label := recoveryLabel(m)
			if promoteBrandRe.MatchString(label) {
				// Brand ke-deteksi → jangan kotorin graph; mark promoted biar ga muter.
				_ = store.SetMistakePromoted(m.ID, 1)
				continue
			}
			ectx, cancel := context.WithTimeout(ctx, 30*time.Second)
			vec, eerr := rc.EmbedText(ectx, "", label)
			cancel()
			if eerr != nil {
				continue // router lagi sibuk → coba lagi tick berikut (belum di-mark)
			}
			h := fnv.New64a()
			h.Write([]byte(strings.ToLower(label)))
			hid := h.Sum64()
			nodeID := fmt.Sprintf("%s/instinct/recov%016x", scope, hid)
			if _, ue := store.UpsertNode(agentdb.CogNode{
				ID: nodeID, Label: label, Type: "instinct",
				WhereDomain: "recovery", SourceKind: "verified", SourceRef: fmt.Sprintf("recov%016x", hid),
				Confidence: 0.85, Status: "active", Embedding: agentdb.Quantize(vec),
			}); ue != nil {
				continue
			}
			if perr := store.SetMistakePromoted(m.ID, int64(hid>>1)); perr != nil {
				log.Printf("[mistake-promote] set promoted gagal (%s id=%d): %v", id, m.ID, perr)
			}
			promoted++
			log.Printf("[mistake-promote] %s mistake#%d (hit=%d) → recovery-instinct", id, m.ID, m.HitCount)
		}
		store.Close()
	}
	return promoted
}

// recoveryLabel — bentuk recovery-instinct DETERMINISTIK dari mistake (no LLM).
// LEAD sama lesson bersih (sinyal embedding fokus, ga ke-dilute boilerplate — kalau
// noisy, recall kalah saing sama 891 instinct existing). Inc-2 = generalisasi via
// dream-digester (buang spesifik path/id).
func recoveryLabel(m agentdb.Mistake) string {
	content := strings.TrimSpace(m.Content)
	title := strings.TrimSpace(m.Title)
	// Content udah bentuk instinct bersih ("WHEN ... -> ...") → pakai LANGSUNG.
	if strings.HasPrefix(strings.ToUpper(content), "WHEN ") && strings.Contains(content, "->") {
		return trimLen(content, 400)
	}
	// Else: bentuk minimal "WHEN <title> -> <lesson>" (boilerplate seminimal mungkin).
	lesson := content
	if lesson == "" {
		lesson = title
	}
	return trimLen("WHEN "+title+" -> "+lesson, 400)
}

func trimLen(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
