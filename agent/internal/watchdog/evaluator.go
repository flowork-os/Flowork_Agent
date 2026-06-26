// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"flowork-gui/internal/agentdb"
)

type Rule struct {
	ID        string
	EventType string
	Threshold int
	Window    time.Duration
	Cooldown  time.Duration
	Severity  string
}

func DefaultRules() []Rule {
	return []Rule{
		{
			ID:        "protector_burst",
			EventType: agentdb.EventProtectorBlock,
			Threshold: 10,
			Window:    60 * time.Second,
			Cooldown:  1 * time.Hour,
			Severity:  agentdb.AuditSevCritical,
		},
		{
			ID:        "scanner_critical_burst",
			EventType: agentdb.EventScannerFinding,
			Threshold: 5,
			Window:    1 * time.Hour,
			Cooldown:  1 * time.Hour,
			Severity:  "high",
		},
		{
			ID:        "tool_call_storm",
			EventType: agentdb.EventToolCall,
			Threshold: 100,
			Window:    60 * time.Second,
			Cooldown:  1 * time.Hour,
			Severity:  agentdb.AuditSevWarning,
		},
	}
}

type AgentEnumerator func() []string
type StoreOpener func(agentID string) (*agentdb.Store, error)
type Notifier func(ctx context.Context, agentID, channel, message string) error

type Engine struct {
	enum     AgentEnumerator
	opener   StoreOpener
	notifier Notifier
	rules    []Rule
	interval time.Duration
	stop     chan struct{}
}

func New(enum AgentEnumerator, opener StoreOpener, notifier Notifier) *Engine {
	return &Engine{
		enum:     enum,
		opener:   opener,
		notifier: notifier,
		rules:    DefaultRules(),
		interval: 60 * time.Second,
	}
}

func (e *Engine) Start(ctx context.Context) {
	e.stop = make(chan struct{})
	log.Printf("[watchdog] engine started — rules=%d interval=%s", len(e.rules), e.interval)
	go e.loop(ctx)
}

func (e *Engine) Stop() {
	if e.stop != nil {
		close(e.stop)
	}
}

func (e *Engine) FireNow(ctx context.Context) (int, int) {
	return e.tick(ctx)
}

func (e *Engine) loop(ctx context.Context) {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stop:
			return
		case <-timer.C:
			e.tick(ctx)
			timer.Reset(e.interval)
		}
	}
}

func (e *Engine) tick(ctx context.Context) (int, int) {
	evaluated := 0
	fired := 0
	for _, agentID := range e.enum() {
		ev, fi := e.tickAgent(ctx, agentID)
		evaluated += ev
		fired += fi
	}
	return evaluated, fired
}

func (e *Engine) tickAgent(ctx context.Context, agentID string) (int, int) {
	store, err := e.opener(agentID)
	if err != nil {
		return 0, 0
	}
	defer store.Close()

	evaluated := 0
	fired := 0
	for _, rule := range e.rules {
		evaluated++

		recentAlerts, _ := store.ListWatchdogAlerts(50)
		recentSkip := false
		for _, a := range recentAlerts {
			if a.RuleID != rule.ID {
				continue
			}
			ft, perr := time.Parse(time.RFC3339, a.FiredAt)
			if perr == nil && time.Since(ft) < rule.Cooldown {
				recentSkip = true
				break
			}
		}
		if recentSkip {
			continue
		}

		cutoff := time.Now().UTC().Add(-rule.Window).Format(time.RFC3339)
		n, cerr := store.CountAuditInWindow(rule.EventType, cutoff)
		if cerr != nil {
			continue
		}
		if n < rule.Threshold {
			continue
		}

		ctxJSON, _ := json.Marshal(map[string]any{
			"rule_id":    rule.ID,
			"event_type": rule.EventType,
			"hit_count":  n,
			"threshold":  rule.Threshold,
			"window":     rule.Window.String(),
			"severity":   rule.Severity,
		})
		_, _ = store.InsertWatchdogAlert(agentdb.WatchdogAlert{
			RuleID:      rule.ID,
			ContextJSON: string(ctxJSON),
		})
		msg := fmt.Sprintf("[watchdog %s] %s: %d %s in %s (≥%d) — %s",
			agentID, rule.ID, n, rule.EventType, rule.Window, rule.Threshold, rule.Severity)
		if e.notifier != nil {
			_ = e.notifier(ctx, agentID, "log", msg)
		} else {
			log.Print(msg)
		}
		fired++
	}
	return evaluated, fired
}
