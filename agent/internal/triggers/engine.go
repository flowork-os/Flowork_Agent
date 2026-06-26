// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/trigger-schedule.md

package triggers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"flowork-gui/internal/floworkdb"
)

type Engine struct {
	Store  *floworkdb.Store
	Invoke func(ctx context.Context, target, text, caller string) (string, error)
	Notify func(ctx context.Context, text string) error

	SystemAction func(ctx context.Context, action string) (string, error)
}

var (
	errNotFound = errors.New("trigger not found / disabled")
	errAuth     = errors.New("invalid webhook key")
	errMode     = errors.New("type is not webhook mode")
)

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
			fresh, _ := e.Store.MarkTriggerKey(r.ID, ev.Key)
			if !fresh {
				continue
			}
			rule, event := r, ev
			go e.runAction(rule, event, "poll")
		}
	}
}

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

func (e *Engine) RunNow(ruleID string) (int64, error) {
	r, err := e.Store.GetTrigger(ruleID)
	if err != nil || r == nil {
		return 0, errNotFound
	}
	ev := Event{Key: "manual", Payload: map[string]string{"manual": "true", "time": time.Now().Format(time.RFC3339)}}
	return e.runAction(*r, ev, "manual"), nil
}

func (e *Engine) runAction(r floworkdb.Trigger, ev Event, trigger string) int64 {
	payloadJSON, _ := json.Marshal(ev.Payload)
	runID, _ := e.Store.InsertTriggerRun(r.ID, trigger, string(payloadJSON))
	prompt := renderTemplate(r.Prompt, ev.Payload)
	cctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	status, errText, reply := "ok", "", ""
	if r.TargetKind == "system" {

		if e.SystemAction == nil {
			status, errText = "error", "system action not wired"
		} else if out, ierr := e.SystemAction(cctx, r.Target); ierr != nil {
			status, errText = "error", ierr.Error()
		} else {
			reply = out
		}
		_ = e.Store.FinishTriggerRun(runID, status, reply, errText)
		_ = e.Store.MarkTriggerFired(r.ID, time.Now().UTC().Format(time.RFC3339), status)
		return runID
	}
	if e.Invoke == nil {
		status, errText = "error", "invoke not wired"
	} else if out, ierr := e.Invoke(cctx, r.Target, prompt, "trigger:"+r.ID); ierr != nil {
		status, errText = "error", ierr.Error()
	} else {
		reply = out

		errText = dispatchDeliver(cctx, e, r, reply)
	}
	_ = e.Store.FinishTriggerRun(runID, status, reply, errText)
	_ = e.Store.MarkTriggerFired(r.ID, time.Now().UTC().Format(time.RFC3339), status)
	return runID
}
