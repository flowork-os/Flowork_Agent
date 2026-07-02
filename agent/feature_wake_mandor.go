// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// 📄 Dok: FLowork_os/lock/worklog.md (§wake-push)
//
// feature_wake_mandor.go — F-C WAKE-PUSH MANDOR (sibling non-frozen, deletable).
// Dulu: mandor kebangun cuma via POLL trigger `worklog-pending` (interval). Sekarang:
// worker SELESAI dikerjain via tool `agent_command` → mandor langsung dibangunin
// by-EVENT (Engine.RunNow rule worklog-pending) — reaksi instan, bukan nunggu poll.
//
// NOL buka frozen: wrap seam `builtins.InvokeAgentFunc` (var Pola B di
// agent_command.go frozen) pas PhaseSeed (setelah main.go wiring). Poll trigger
// TETAP jalan (jaring pengaman buat task nyangkut); wake-push cuma nambah kecepatan.
// Debounce 60 detik — N worker kelar beruntun = 1 wake (anti spam mandor).
// Anti self-loop: rule yang target-nya = worker yang barusan kelar di-skip.
// Switch GUI FLOWORK_WAKE_MANDOR (default ON). Hapus file ini → balik poll-only.
package main

import (
	"context"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/tools/builtins"
)

// wakeMandorEnabled — switch GUI FLOWORK_WAKE_MANDOR (default ON).
func wakeMandorEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("FLOWORK_WAKE_MANDOR")))
	return v != "0" && v != "false" && v != "off"
}

const wakeMandorDebounceMs = 60_000

var wakeMandorLastMs int64 // unix ms wake terakhir (debounce, atomic)

// wakeMandorDue — gerbang debounce: true kalau udah lewat jendela & berhasil klaim slot.
func wakeMandorDue(nowMs int64) bool {
	last := atomic.LoadInt64(&wakeMandorLastMs)
	if nowMs-last < wakeMandorDebounceMs {
		return false
	}
	return atomic.CompareAndSwapInt64(&wakeMandorLastMs, last, nowMs)
}

// wakeMandorRuleIDs — pilih rule worklog-pending yang layak dibangunin: enabled +
// bukan nge-target worker yang barusan selesai (anti self-loop).
func wakeMandorRuleIDs(rules []floworkdb.Trigger, workerID string) []string {
	ids := []string{}
	for _, r := range rules {
		if r.TypeID != "worklog-pending" || !r.Enabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(r.Target), strings.TrimSpace(workerID)) {
			continue
		}
		ids = append(ids, r.ID)
	}
	return ids
}

// wakeMandorRuleSeedID — rule worklog→mandor (by-kerjaan). Sibling dari idle-mandor
// (feature_mandor.go). Poll = jaring pengaman; wake-push (di bawah) = jalur cepatnya.
const wakeMandorRuleSeedID = "worklog-mandor"

// seedWorklogMandorRule — idempotent (skip kalau ID udah ada → hormatin edit/disable
// owner di GUI). Gate FLOWORK_MANDOR sama kayak seed idle-mandor.
func seedWorklogMandorRule(fdb *floworkdb.Store) {
	if fdb == nil {
		return
	}
	if existing, err := fdb.ListTriggers(); err == nil {
		for _, t := range existing {
			if t.ID == wakeMandorRuleSeedID {
				return
			}
		}
	}
	_ = fdb.UpsertTrigger(floworkdb.Trigger{
		ID:         wakeMandorRuleSeedID,
		Name:       "Mandor — kerjaan nyangkut (worklog)",
		TypeID:     "worklog-pending",
		Config:     `{"cooldown_min":"30"}`,
		Target:     "mandor",
		TargetKind: "agent",
		Prompt: "Ada {{pending}} kerjaan nyangkut / worker baru selesai. Jalanin tugas MANDOR kamu: " +
			"panggil tool `worklog`, fokus yang priority=high + stale, lalu `agent_command` ke agent " +
			"pemiliknya buat lanjut. Kalau papan kosong / ga ada yang nyangkut → diem, balas singkat 'aman'.",
		Deliver: "",
		Enabled: true,
	})
}

func init() {
	RegisterFeature(Feature{Name: "wake-mandor", Phase: PhaseSeed, Apply: func(d *Deps) {
		if d.TrigEngine == nil || d.FDB == nil {
			return
		}
		if mandorEnabled() {
			seedWorklogMandorRule(d.FDB)
		}
		inner := builtins.InvokeAgentFunc
		if inner == nil {
			return // agent_command belum di-wire → ga ada sinyal buat di-hook
		}
		builtins.InvokeAgentFunc = func(ctx context.Context, agentID, text, caller string) (string, error) {
			reply, err := inner(ctx, agentID, text, caller)
			if err == nil && wakeMandorEnabled() {
				worker := agentID
				go func() {
					rules, lerr := d.FDB.ListTriggers()
					if lerr != nil {
						return
					}
					ids := wakeMandorRuleIDs(rules, worker)
					if len(ids) == 0 {
						return // ga ada rule layak → JANGAN bakar slot debounce
					}
					if !wakeMandorDue(time.Now().UnixMilli()) {
						return
					}
					for _, id := range ids {
						if runID, rerr := d.TrigEngine.RunNow(id); rerr == nil {
							log.Printf("wake-mandor: worker %q selesai → RunNow rule %s (run %d)", worker, id, runID)
						}
					}
				}()
			}
			return reply, err
		}
	}})
}
