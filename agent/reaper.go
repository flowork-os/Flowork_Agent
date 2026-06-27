// reaper.go — REAPER / Apoptosis (roadmap 2.4). Imun beneran BIKIN dan BUNUH.
// Coder bikin agent terus → sprawl. Reaper cabut app ber-"karma rendah" (error-
// rate tinggi) / broken (synth ga load). Prinsip "agent bodoh engine pinter":
// sinyal DETERMINISTIK dari data NYATA (task_runs done/error + smoke), BUKAN LLM.
//
//	GET  /api/reaper/candidates       → daftar app + health + flag candidate
//	POST /api/reaper/reap?category=<> → owner-gated reap → uninstall via pipeline
//
// Owner-gated (sama kayak Coder Approval Queue): Reaper SURFACE candidate,
// MANUSIA yang mutusin cabut. Loopback-only.

package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernelhost"
)

// reap threshold — app dianggap "failing" kalau error-rate > 40% DAN sampel
// cukup (>=5 run selesai). Interrupted (user cancel) GA dihitung gagal.
// SWITCH (peta-saraf, default = perilaku lama): owner bisa setel keagresifan vonis Reaper
// per-fleet tanpa edit kode — FLOWORK_REAP_ERRRATE (0..1) + FLOWORK_REAP_MIN_SAMPLES (>0).
func reapErrRateThreshold() float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("FLOWORK_REAP_ERRRATE")), 64); err == nil && v > 0 && v <= 1 {
		return v
	}
	return 0.40
}

func reapMinSamples() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FLOWORK_REAP_MIN_SAMPLES"))); err == nil && n > 0 {
		return n
	}
	return 5
}

// ReapCandidate — health 1 app + verdict reaper.
type ReapCandidate struct {
	CategoryID string  `json:"category_id"`
	Name       string  `json:"name"`
	Done       int     `json:"done"`
	Error      int     `json:"error"`
	ErrorRate  float64 `json:"error_rate"`
	Smoke      string  `json:"smoke"` // ok | llm_error | not_loaded | "-"
	Flagged    bool    `json:"flagged"`
	// ReasonCode = enum (healthy|broken|failing) — GUI render teks lokal via
	// dictionary (no hardcode bahasa di backend yang bocor ke GUI).
	ReasonCode string `json:"reason_code"`
	Severity   string `json:"severity"` // healthy | warn | critical
}

// reapScan — health SEMUA kategori. Flag yang broken (synth ga load = critical)
// atau failing (error-rate tinggi = warn). App sehat tetep dilist (transparansi).
func reapScan(host *kernelhost.Host, store *floworkdb.Store) ([]ReapCandidate, error) {
	cats, err := store.ListCategories()
	if err != nil {
		return nil, err
	}
	stats, err := store.CategoryRunStats()
	if err != nil {
		return nil, err
	}
	// smoke-test per kategori = invoke synth (lambat) → JALANIN PARALEL. Tiap
	// goroutine nulis ke slot index sendiri (out[i]) → no race. ~10 kategori
	// barengan: ~11s → ~1-2s. Cap concurrency biar ga spike N agent sekaligus.
	out := make([]ReapCandidate, len(cats))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // max 8 smoke barengan
	for i := range cats {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c := cats[i]
			st := stats[c.ID]
			closed := st.Done + st.Error
			rate := 0.0
			if closed > 0 {
				rate = float64(st.Error) / float64(closed)
			}
			cand := ReapCandidate{
				CategoryID: c.ID, Name: c.Name, Done: st.Done, Error: st.Error,
				ErrorRate: rate, Smoke: "-", Severity: "healthy", ReasonCode: "healthy",
			}
			// smoke: synth masih ke-load? (broken = paling parah)
			full, _ := store.GetCategory(c.ID)
			if full != nil && full.Synthesizer != "" {
				cand.Smoke = smokeTestSynth(host, full.Synthesizer)
			}
			switch {
			case cand.Smoke == "not_loaded":
				cand.Flagged, cand.Severity, cand.ReasonCode = true, "critical", "broken"
			case closed >= reapMinSamples() && rate > reapErrRateThreshold():
				cand.Flagged, cand.Severity, cand.ReasonCode = true, "warn", "failing"
			}
			out[i] = cand
		}(i)
	}
	wg.Wait()
	return out, nil
}

func reaperCandidatesHandler(host *kernelhost.Host, store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cands, err := reapScan(host, store)
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		flagged := 0
		for _, c := range cands {
			if c.Flagged {
				flagged++
			}
		}
		tfWriteJSON(w, 0, map[string]any{"candidates": cands, "flagged": flagged, "total": len(cands)})
	}
}

// reaperReapHandler — POST ?category=. Owner-gated reap → uninstall via pipeline
// yg UDAH ada (uninstallCategoryCore). Reaper ga maksa — owner yg klik.
func reaperReapHandler(store *floworkdb.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		body, status := uninstallCategoryCore(store, r.URL.Query().Get("category"), "reaped (owner-approved: low-karma/broken)")
		if status == 0 {
			body["reaped"] = r.URL.Query().Get("category")
		}
		tfWriteJSON(w, status, body)
	}
}
