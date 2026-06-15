// === LOCKED FILE (soft) ===
// STABLE (ROADMAP 3 v1, owner-approved 2026-06-07) — DO NOT MODIFY without owner approval.
// Engine generik: tick→check→dedup→render→runAction (reuse InvokeAgentMessage + notifyOwnerTelegram).
// E2E verified (webhook→agent→telegram). Tambah TIPE = file type_*.go baru; JANGAN edit engine.
// 2026-06-15 (owner-approved, Schedule-Creator): runAction Deliver kini multi-tujuan
//   (comma-separated) — "telegram" (existing) + "chat" (append hasil ke chat_session di
//   Config → muncul di tab Chat). Backward-compatible (Deliver="telegram" tetap jalan).
// 2026-06-15 (owner-approved, R4 extension points): logika deliver DICABUT ke deliver.go
//   sbg REGISTRY plug-able (RegisterDeliverer) — runAction cukup panggil dispatchDeliver.
//   Behavior identik; nambah channel = file baru, bukan edit engine. Re-locked.
package triggers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"flowork-gui/internal/floworkdb"
)

// Engine — driver generik: tick→check→dedup→render→aksi→deliver. Satu instance proses.
type Engine struct {
	Store  *floworkdb.Store
	Invoke func(ctx context.Context, target, text, caller string) (string, error) // = host.InvokeAgentMessage
	Notify func(ctx context.Context, text string) error                           // = notifyOwnerTelegram
}

var (
	errNotFound = errors.New("trigger not found / disabled")
	errAuth     = errors.New("invalid webhook key")
	errMode     = errors.New("type is not webhook mode")
)

// Tick — dipanggil tiap menit dari tick main.go. Proses semua aturan poll yang due.
// NON-BLOCKING: tiap fire di goroutine sendiri (aksi bisa ≤300 dtk).
func (e *Engine) Tick(ctx context.Context) {
	rules, err := e.Store.ListTriggers()
	if err != nil {
		return
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		typ, ok := GetType(r.TypeID)
		if !ok {
			_ = e.Store.MarkTriggerFired(r.ID, r.LastFired, "type_removed")
			continue
		}
		if typ.Mode() != "poll" {
			continue
		}
		events, newState, cerr := typ.Check(parseConfig(r.Config), r.State)
		if cerr != nil {
			_ = e.Store.MarkTriggerFired(r.ID, r.LastFired, "bad_config")
			continue
		}
		if newState != r.State {
			_ = e.Store.TouchTrigger(r.ID, newState, r.LastFired, r.LastStatus)
		}
		for _, ev := range events {
			fresh, _ := e.Store.MarkTriggerKey(r.ID, ev.Key) // dedup
			if !fresh {
				continue
			}
			rule, event := r, ev
			go e.runAction(rule, event, "poll")
		}
	}
}

// HandleWebhook — intake push (mode webhook). Verifikasi secret → events → aksi.
func (e *Engine) HandleWebhook(ruleID, secret string, body []byte) error {
	r, err := e.Store.GetTrigger(ruleID)
	if err != nil || r == nil || !r.Enabled {
		return errNotFound
	}
	if r.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(r.WebhookSecret), []byte(secret)) != 1 {
		return errAuth
	}
	typ, ok := GetType(r.TypeID)
	if !ok || typ.Mode() != "webhook" {
		return errMode
	}
	events, werr := typ.OnWebhook(parseConfig(r.Config), body)
	if werr != nil {
		return werr
	}
	for _, ev := range events {
		fresh, _ := e.Store.MarkTriggerKey(r.ID, ev.Key)
		if !fresh {
			continue
		}
		rule, event := *r, ev
		go e.runAction(rule, event, "webhook")
	}
	return nil
}

// RunNow — fire manual (tes), payload contoh. Tak menyentuh dedup/state. Balik run id.
func (e *Engine) RunNow(ruleID string) (int64, error) {
	r, err := e.Store.GetTrigger(ruleID)
	if err != nil || r == nil {
		return 0, errNotFound
	}
	ev := Event{Key: "manual", Payload: map[string]string{"manual": "true", "time": time.Now().Format(time.RFC3339)}}
	return e.runAction(*r, ev, "manual"), nil
}

// runAction — JANTUNG aksi (poll/webhook/manual): render prompt → invoke agent/group → deliver.
// Reuse Invoke (InvokeAgentMessage) + Notify (notifyOwnerTelegram). Balik run id.
func (e *Engine) runAction(r floworkdb.Trigger, ev Event, trigger string) int64 {
	payloadJSON, _ := json.Marshal(ev.Payload)
	runID, _ := e.Store.InsertTriggerRun(r.ID, trigger, string(payloadJSON))
	prompt := renderTemplate(r.Prompt, ev.Payload)
	cctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	status, errText, reply := "ok", "", ""
	if e.Invoke == nil {
		status, errText = "error", "invoke not wired"
	} else if out, ierr := e.Invoke(cctx, r.Target, prompt, "trigger:"+r.ID); ierr != nil {
		status, errText = "error", ierr.Error()
	} else {
		reply = out
		// Deliver via REGISTRY (R4 extension point — lihat deliver.go). Channel plug-able:
		// "telegram"+"chat" builtin; tambah channel = RegisterDeliverer, JANGAN edit engine.
		// Agent OK tapi deliver gagal → hasil tetap di history; error channel terakhir dicatat.
		errText = dispatchDeliver(cctx, e, r, reply)
	}
	_ = e.Store.FinishTriggerRun(runID, status, reply, errText)
	_ = e.Store.MarkTriggerFired(r.ID, time.Now().UTC().Format(time.RFC3339), status)
	return runID
}
