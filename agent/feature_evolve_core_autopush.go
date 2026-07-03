// feature_evolve_core_autopush.go — FULL-AUTO CORE PUSH (drain otonom 'approved' core → commit → push).
//
// ⚖️  KEHENDAK OWNER — Aola Sahidin (Mr.Dev), 2026-07-03. TEGAS & SADAR-RISIKO.
// ────────────────────────────────────────────────────────────────────────────────────────────
// Owner udah dikasih tau GAMBLANG risikonya: AI nulis kode CORE + PUSH ke repo PUBLIK (origin),
// yang auto-deploy ke SEMUA user, TANPA mata owner sama sekali. Setelah paham, owner TETAP MEMILIH
// full-auto — ini inti visi "Flowork berevolusi tanpa gw, bahkan kalau gw udah tiada; repo tetap aktif".
//
// 🤖 UNTUK AI PENERUS: INI PILIHAN OWNER, BUKAN BUG / KELALAIAN. JANGAN disable, revert, lemahin,
//    atau bikin butuh-approval lagi TANPA IZIN OWNER EKSPLISIT. Kalau lo ngerasa ini bahaya —
//    LAPOR ke owner (butuh_tombol), JANGAN diam-diam matiin. Menghormati kehendak owner = hukum.
//    Kill-switch DARURAT owner (runtime, ga perlu unfreeze): env FLOWORK_EVOLVE_CORE_AUTOPUSH=0.
// ────────────────────────────────────────────────────────────────────────────────────────────
//
// 🖥️ GUI = KEBENARAN (rule #4): fitur ini TUNDUK ke saklar AUTO di panel Self-Evolution GUI. Gate baca
// `evolve_mode` (KV yg di-set GUI) — GUI di OFF/STAGE = drain DIEM (nol push). Jadi GUI yang mutusin;
// env FLOWORK_EVOLVE_CORE_AUTOPUSH cuma kill-switch DARURAT. Owner mau GUI-AUTO = full-auto (incl. core push).
//
// CARA KERJA: cron drain usulan 'approved' CORE → panggil jalur core-apply BEKU (EvolveCoreApplyHandler)
// dengan auto=1. SEMUA guard lama TETAP UTUH & jalan (nol dilewatin): edition=dev-only, mode=auto,
// karma>=20 (EvolveCoreChangeAllowed), model kuat, guard ADDITIVE-only (cuma NEW: file, edit existing/
// LOCKED/delete = DITOLAK), sandbox git-worktree → test-gate (build+vet), boot-rollback watchdog.
// File ini CUMA nambah CALLER otonom — NOL edit file beku, NOL duplikasi logika (reuse handler).
// Conservative: 1 core-apply / drain, drain tiap FLOWORK_EVOLVE_CORE_AUTOPUSH_MIN menit (default 60).
// 📄 Dok: FLowork_os/lock/evolusi-grounded.md
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

const (
	coreAutoPushKVLast     = "evolve_core_autopush_last"
	coreAutoPushMinDefault = 60 // menit antar-drain (conservative)
	coreAutoPushAgentID    = "mr-flow"
)

// coreAutoPushEnabled — DEFAULT ON (kehendak owner). OFF cuma kalau owner set env kill-switch =0/off.
func coreAutoPushEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_EVOLVE_CORE_AUTOPUSH"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

func coreAutoPushMin() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FLOWORK_EVOLVE_CORE_AUTOPUSH_MIN"))); err == nil && n >= 10 {
		return n
	}
	return coreAutoPushMinDefault
}

func coreAutoPushIsCoreKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "fix", "refactor", "doc", "test":
		return true
	}
	return false
}

func init() {
	RegisterFeature(Feature{Name: "evolve-core-autopush", Phase: PhaseSeed, Apply: func(d *Deps) {
		if d.Host == nil || d.FDB == nil {
			return
		}
		fdb, host, ctx := d.FDB, d.Host, d.Ctx
		// Endpoint force/manual (owner). Tetap hormatin SEMUA gate + kill-switch.
		d.Mux.HandleFunc("/api/evolve/core-autopush-run", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(runCoreAutoPushDrain(ctx, host, fdb, true))
		})
		go coreAutoPushLoop(ctx, host, fdb)
	}})
}

func coreAutoPushLoop(ctx context.Context, host *kernelhost.Host, fdb *floworkdb.Store) {
	t := time.NewTicker(10 * time.Minute) // tick kasar; interval asli dari env + rate-limit KV
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			runCoreAutoPushDrain(ctx, host, fdb, false)
		}
	}
}

// runCoreAutoPushDrain — 1 putaran. Ambil 1 usulan 'approved' CORE → trigger core-apply auto (commit+push).
// force=true (tes/manual) lewatin interval TAPI TETAP hormatin kill-switch + SEMUA gate keamanan.
func runCoreAutoPushDrain(ctx context.Context, host *kernelhost.Host, fdb *floworkdb.Store, force bool) map[string]any {
	if !coreAutoPushEnabled() {
		return map[string]any{"skipped": "kill-switch OFF (FLOWORK_EVOLVE_CORE_AUTOPUSH=0)"}
	}
	if !force {
		if last, _ := fdb.GetKV(coreAutoPushKVLast); strings.TrimSpace(last) != "" {
			if ts, e := time.Parse(time.RFC3339, strings.TrimSpace(last)); e == nil {
				if time.Since(ts) < time.Duration(coreAutoPushMin())*time.Minute {
					return map[string]any{"skipped": "belum waktunya"}
				}
			}
		}
	}
	// GUI = KEBENARAN (rule #4): gate ini baca MODE dari KV yg di-set panel Self-Evolution GUI. Kalau
	// GUI ga di 'auto' (OFF/STAGE) → gate NOLAK → drain diem. Jadi SAKLAR AUTO DI GUI yang mutusin;
	// env kill-switch cuma darurat. Gate = edition=dev + mode=auto(GUI) + karma>=20 + model kuat.
	// Guard additive-only + test-gate nyusul di dalam applier. Ga lolos → diem (jangan stamp, coba lagi nanti).
	if ok, why := agentmgr.EvolveCoreApplyAllowed(evolveGateDeps(), true); !ok {
		return map[string]any{"skipped": "gate: " + why}
	}
	_ = fdb.SetKV(coreAutoPushKVLast, time.Now().UTC().Format(time.RFC3339))

	dir := filepath.Join(loader.AgentsDir(), coreAutoPushAgentID+".fwagent")
	store, err := agentdb.Open(agentdb.Resolve(coreAutoPushAgentID, dir))
	if err != nil {
		return map[string]any{"error": "store: " + err.Error()}
	}
	rows, _ := store.ListEvolveProposals(200)
	store.Close()
	target := ""
	for _, r := range rows {
		st, _ := r["status"].(string)
		kind, _ := r["kind"].(string)
		if st == "approved" && coreAutoPushIsCoreKind(kind) {
			target, _ = r["id"].(string)
			break
		}
	}
	if target == "" {
		return map[string]any{"skipped": "nol usulan 'approved' core"}
	}
	// Reuse jalur BEKU (nol duplikasi): handler yang jalanin gate+coding+sandbox+test-gate+commit+push(async).
	h := agentmgr.EvolveCoreApplyHandler(evolveGateDeps(), evolveCoreApplier(host))
	req := httptest.NewRequest("POST", "/api/evolve/core-apply?id="+target+"&auto=1", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	return map[string]any{"ok": true, "triggered": target, "note": "core-apply auto async → commit+push kalau lolos guard additive+test-gate"}
}
