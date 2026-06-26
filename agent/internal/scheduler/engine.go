// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"flowork-gui/internal/agentdb"
)

type AgentEnumerator func() []string

type StoreOpener func(agentID string) (*agentdb.Store, error)

type Executor func(ctx context.Context, agentID, scheduleID, task string) (string, error)

type Engine struct {
	mu       sync.Mutex
	running  bool
	enum     AgentEnumerator
	opener   StoreOpener
	executor Executor
	interval time.Duration
	stop     chan struct{}
}

func New(enum AgentEnumerator, opener StoreOpener, executor Executor) *Engine {
	return &Engine{
		enum:     enum,
		opener:   opener,
		executor: executor,
		interval: 60 * time.Second,
	}
}

func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stop = make(chan struct{})
	e.mu.Unlock()
	log.Printf("[scheduler] engine started — tick interval %s", e.interval)
	go e.loop(ctx)
}

func (e *Engine) FireNow(ctx context.Context, agentID, scheduleID string) (int64, error) {
	store, err := e.opener(agentID)
	if err != nil {
		return 0, err
	}
	defer store.Close()
	if err := store.SchedulerSchemaInit(); err != nil {
		return 0, err
	}
	rows, err := store.ListSchedulesForRunner()
	if err != nil {
		return 0, err
	}
	var target *agentdb.ScheduleRow
	for i := range rows {
		if rows[i].ID == scheduleID {
			target = &rows[i]
			break
		}
	}
	if target == nil {
		return 0, fmt.Errorf("schedule %q not found", scheduleID)
	}
	now := time.Now().UTC()
	startedAt := agentdb.AbsTime(now)
	result, runErr := e.executor(ctx, agentID, target.ID, target.Task)
	finishedAt := time.Now().UTC()
	status := "success"
	errText := ""
	if runErr != nil {
		status = "fail"
		errText = runErr.Error()
	}
	id, ierr := store.InsertSchedulerRun(agentdb.SchedulerRun{
		ScheduleID: target.ID,
		Cron:       target.Cron,
		Task:       target.Task,
		StartedAt:  startedAt,
		FinishedAt: agentdb.AbsTime(finishedAt),
		DurationMS: finishedAt.Sub(now).Milliseconds(),
		Status:     status,
		ResultText: truncate(result, 16*1024),
		ErrorText:  truncate(errText, 4*1024),
	})
	if ierr != nil {
		return 0, ierr
	}
	return id, nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return
	}
	close(e.stop)
	e.running = false
}

func (e *Engine) loop(ctx context.Context) {

	now := time.Now().UTC()
	delay := time.Duration(60-now.Second())*time.Second -
		time.Duration(now.Nanosecond())*time.Nanosecond
	if delay < 100*time.Millisecond {
		delay = 100 * time.Millisecond
	}
	timer := time.NewTimer(delay)
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

func (e *Engine) tick(ctx context.Context) {
	now := time.Now().UTC()
	agentIDs := e.enum()
	for _, agentID := range agentIDs {
		go e.tickAgent(ctx, agentID, now)
	}
}

func (e *Engine) tickAgent(ctx context.Context, agentID string, now time.Time) {
	store, err := e.opener(agentID)
	if err != nil {
		log.Printf("[scheduler] open store %s: %v", agentID, err)
		return
	}
	defer store.Close()

	if err := store.SchedulerSchemaInit(); err != nil {
		log.Printf("[scheduler] schema init %s: %v", agentID, err)
		return
	}

	rows, err := store.ListSchedulesForRunner()
	if err != nil {
		log.Printf("[scheduler] list %s: %v", agentID, err)
		return
	}
	for _, r := range rows {
		if !r.Enabled || r.Cron == "" || r.Task == "" {
			continue
		}
		spec, perr := Parse(r.Cron)
		if perr != nil {
			log.Printf("[scheduler] parse %s/%s cron %q: %v", agentID, r.ID, r.Cron, perr)
			continue
		}
		if !spec.Matches(now) {
			continue
		}
		go e.execute(ctx, agentID, r, spec, now)
	}
}

func (e *Engine) execute(ctx context.Context, agentID string, sched agentdb.ScheduleRow, spec Spec, firedAt time.Time) {
	store, oerr := e.opener(agentID)
	if oerr != nil {
		log.Printf("[scheduler] reopen %s: %v", agentID, oerr)
		return
	}
	defer store.Close()

	startedAt := agentdb.AbsTime(firedAt)
	runID, _ := store.InsertSchedulerRun(agentdb.SchedulerRun{
		ScheduleID: sched.ID,
		Cron:       sched.Cron,
		Task:       sched.Task,
		StartedAt:  startedAt,
		Status:     "pending",
	})

	execCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	result, err := e.executor(execCtx, agentID, sched.ID, sched.Task)

	finishedAt := time.Now().UTC()
	duration := finishedAt.Sub(firedAt).Milliseconds()

	status := "success"
	errText := ""
	if err != nil {
		status = "fail"
		errText = err.Error()
	}

	finalRun := agentdb.SchedulerRun{
		ScheduleID: sched.ID,
		Cron:       sched.Cron,
		Task:       sched.Task,
		StartedAt:  startedAt,
		FinishedAt: agentdb.AbsTime(finishedAt),
		DurationMS: duration,
		Status:     status,
		ResultText: truncate(result, 16*1024),
		ErrorText:  truncate(errText, 4*1024),
	}

	_ = runID
	_, _ = store.InsertSchedulerRun(finalRun)

	if next, nerr := spec.Next(firedAt); nerr == nil {
		_ = store.UpdateScheduleRunTime(sched.ID,
			startedAt, agentdb.AbsTime(next))
	} else {
		_ = store.UpdateScheduleRunTime(sched.ID, startedAt, "")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]"
}
