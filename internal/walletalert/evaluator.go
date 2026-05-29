// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 22 phase 2 — cron evaluator yang fire alert kalau
//   wallet balance breached threshold. Multi-warga loop via AgentIDs
//   callback. 1-jam cooldown anti-spam. Phase 3 (per-metric extension
//   beyond total_usd, Discord/email channels, alert priority chain) →
//   tambah file baru.
//
// evaluator.go — Section 22 phase 2: wallet alert cron evaluator.

package walletalert

import (
	"context"
	"fmt"
	"log"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/wallet"
)

// AgentEnumerator + StoreOpener + Notifier — callbacks dari main.go.
type AgentEnumerator func() []string
type StoreOpener func(agentID string) (*agentdb.Store, error)

// Notifier — dispatch alert. Caller (main) inject implementation yang
// tau cara reach Telegram per agent (via tool / direct API).
type Notifier func(ctx context.Context, agentID, channel, target, message string) error

// Engine — top-level. interval default 1 jam.
type Engine struct {
	enum     AgentEnumerator
	opener   StoreOpener
	notifier Notifier
	interval time.Duration
	stop     chan struct{}
	cooldown time.Duration // anti-spam — alert fire ulang max tiap N
}

// New — caller wajib supply 3 callback.
func New(enum AgentEnumerator, opener StoreOpener, notifier Notifier) *Engine {
	return &Engine{
		enum:     enum,
		opener:   opener,
		notifier: notifier,
		interval: 1 * time.Hour,
		cooldown: 24 * time.Hour, // per acceptance criteria Section 22
	}
}

// Start — spawn goroutine. ctx cancellation hentikan.
func (e *Engine) Start(ctx context.Context) {
	e.stop = make(chan struct{})
	log.Printf("[walletalert] engine started — eval interval %s, cooldown %s", e.interval, e.cooldown)
	go e.loop(ctx)
}

// Stop — signal goroutine.
func (e *Engine) Stop() {
	if e.stop != nil {
		close(e.stop)
	}
}

// FireNow — trigger sweep sekali (admin manual via endpoint). Useful buat test.
func (e *Engine) FireNow(ctx context.Context) (int, int) {
	return e.tick(ctx)
}

func (e *Engine) loop(ctx context.Context) {
	timer := time.NewTimer(30 * time.Second) // first eval after 30s warm-up
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

// tick — iterate agents, eval alerts, fire jika perlu. Return (evaluated, fired).
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
		log.Printf("[walletalert] open %s: %v", agentID, err)
		return 0, 0
	}
	defer store.Close()

	configs, cerr := store.ListWalletAlerts()
	if cerr != nil {
		log.Printf("[walletalert] list configs %s: %v", agentID, cerr)
		return 0, 0
	}
	if len(configs) == 0 {
		return 0, 0
	}
	addresses, _ := store.ListWalletAddresses()
	if len(addresses) == 0 {
		return 0, 0
	}

	// Phase 2: cuma metric "total_usd" yg di-evaluasi. Fetch portfolio per
	// address, aggregate sum (single-owner reality biasanya 1 address).
	var aggregateUSD float64
	for _, addr := range addresses {
		portfolio, perr := wallet.Snapshot(ctx, addr.Address)
		if perr != nil {
			log.Printf("[walletalert] snapshot %s %s: %v (skip address)", agentID, addr.Address, perr)
			continue
		}
		aggregateUSD += portfolio.TotalUSD
	}

	evaluated := 0
	fired := 0
	for _, cfg := range configs {
		evaluated++
		if !cfg.Enabled {
			continue
		}
		if cfg.MetricKey != "total_usd" {
			continue // phase 3 extension
		}
		if !breached(aggregateUSD, cfg.Comparator, cfg.ThresholdValue) {
			continue
		}
		// Cooldown check: last_fired_at < N → skip.
		if cfg.LastFiredAt != "" {
			lastFired, perr := time.Parse(time.RFC3339, cfg.LastFiredAt)
			if perr == nil && time.Since(lastFired) < e.cooldown {
				continue
			}
		}
		// Fire.
		msg := fmt.Sprintf("[%s] total_usd=%.2f breached %s %.2f (alert_id=%d)",
			agentID, aggregateUSD, cfg.Comparator, cfg.ThresholdValue, cfg.ID)
		if e.notifier != nil {
			if nerr := e.notifier(ctx, agentID, cfg.NotifyChannel, cfg.NotifyTarget, msg); nerr != nil {
				log.Printf("[walletalert] notify %s: %v", agentID, nerr)
			}
		} else {
			log.Printf("[walletalert] %s", msg)
		}
		if ierr := store.InsertWalletAlertFired(cfg.ID, aggregateUSD, msg); ierr != nil {
			log.Printf("[walletalert] insert fired %s: %v", agentID, ierr)
		}
		fired++
	}
	if evaluated > 0 || fired > 0 {
		log.Printf("[walletalert] %s eval=%d fire=%d total_usd=%.2f",
			agentID, evaluated, fired, aggregateUSD)
	}
	return evaluated, fired
}

func breached(actual float64, comparator string, threshold float64) bool {
	switch comparator {
	case "<":
		return actual < threshold
	case "<=":
		return actual <= threshold
	case ">":
		return actual > threshold
	case ">=":
		return actual >= threshold
	}
	return false
}
