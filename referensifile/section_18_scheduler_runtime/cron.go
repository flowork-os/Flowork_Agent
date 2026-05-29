package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/fsutil"
	"github.com/teetah2402/flowork/internal/provider"
)

// ─── Cron registry (persisted to ~/.flowork/cron.json) ─────────────

type CronJob struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"` // 5-field standard cron
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at" validate:"required"`
	LastRun   time.Time `json:"last_run,omitempty"`
	RunCount  int       `json:"run_count" validate:"required"`
}

type CronStore struct {
	mu   sync.Mutex
	path string
	jobs map[string]CronJob
}

// Global singleton — shared across cron_create/delete/list to prevent memory
// desync and concurrent file-write races (Gemini audit 2026-04-15).
var (
	globalCronStoreOnce sync.Once
	globalCronStore     *CronStore
)

func NewCronStore() *CronStore {
	globalCronStoreOnce.Do(func() {
		home, _ := os.UserHomeDir()
		path := filepath.Join(home, ".flowork", "cron.json")
		globalCronStore = &CronStore{path: path, jobs: make(map[string]CronJob)}
		globalCronStore.load()
	})
	return globalCronStore
}

func (s *CronStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var arr []CronJob
	if err := json.Unmarshal(data, &arr); err != nil {
		return
	}
	for _, j := range arr {
		s.jobs[j.ID] = j
	}
}

func (s *CronStore) save() error {
	_ = os.MkdirAll(filepath.Dir(s.path), 0755)
	arr := make([]CronJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		arr = append(arr, j)
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].CreatedAt.Before(arr[j].CreatedAt) })
	data, _ := json.MarshalIndent(arr, "", "  ")
	// EXTBUG-017 fix: atomic tmp+rename + 0600. Crash mid-write no longer
	// corrupts scheduled tasks, and perms don't expose cron payload bodies
	// (may contain task descriptions with secrets) to other local users.
	return fsutil.WriteFileAtomic(s.path, data, 0o600)
}

// ─── cron_create ───────────────────────────────────────────────────

type CronCreateTool struct{ store *CronStore }

func NewCronCreateTool() *CronCreateTool { return &CronCreateTool{store: NewCronStore()} }

type cronCreateArgs struct {
	Schedule string `json:"schedule"`
	Prompt   string `json:"prompt" validate:"required"`
	Days     int    `json:"days,omitempty"`
}

func (t *CronCreateTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "cron_create",
		Description: "Create a scheduled prompt to run on a 5-field cron (minute hour day month weekday). Auto-expires in 7 days unless days override. Returns job ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"schedule": map[string]any{"type": "string", "description": "5-field cron, e.g. '0 9 * * 1-5' for weekdays at 9am"},
				"prompt":   map[string]any{"type": "string", "description": "Prompt to run when cron fires"},
				"days":     map[string]any{"type": "integer", "description": "Expire after N days (default 7)"},
			},
			"required": []string{"schedule", "prompt"},
		},
	}
}

func (t *CronCreateTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args cronCreateArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Schedule) == "" || strings.TrimSpace(args.Prompt) == "" {
		return Result{}, fmt.Errorf("schedule and prompt required")
	}
	if args.Days <= 0 {
		args.Days = 7
	}
	id := fmt.Sprintf("cron-%d", time.Now().UnixNano())
	job := CronJob{
		ID:        id,
		Schedule:  args.Schedule,
		Prompt:    args.Prompt,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(args.Days) * 24 * time.Hour),
	}
	t.store.mu.Lock()
	t.store.jobs[id] = job
	err := t.store.save()
	t.store.mu.Unlock()
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:   fmt.Sprintf("Created cron %s (schedule: %s, expires: %s)", id, args.Schedule, job.ExpiresAt.Format("2006-01-02")),
		Metadata: map[string]any{"id": id, "schedule": args.Schedule, "expires_at": job.ExpiresAt},
	}, nil
}

// ─── cron_delete ───────────────────────────────────────────────────

type CronDeleteTool struct{ store *CronStore }

func NewCronDeleteTool() *CronDeleteTool { return &CronDeleteTool{store: NewCronStore()} }

type cronDeleteArgs struct {
	ID string `json:"id" validate:"required"`
}

func (t *CronDeleteTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "cron_delete",
		Description: "Delete a scheduled cron job by ID.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"id": map[string]any{"type": "string"}},
			"required":   []string{"id"},
		},
	}
}

func (t *CronDeleteTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args cronDeleteArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if _, ok := t.store.jobs[args.ID]; !ok {
		return Result{}, fmt.Errorf("cron %q not found", args.ID)
	}
	delete(t.store.jobs, args.ID)
	_ = t.store.save()
	return Result{Output: "Deleted cron " + args.ID}, nil
}

// ─── cron_list ─────────────────────────────────────────────────────

type CronListTool struct{ store *CronStore }

func NewCronListTool() *CronListTool { return &CronListTool{store: NewCronStore()} }

func (t *CronListTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "cron_list",
		Description: "List all active cron jobs. Shows ID, schedule, prompt preview, next expiry.",
		InputSchema: map[string]any{"type": "object"},
	}
}

func (t *CronListTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if len(t.store.jobs) == 0 {
		return Result{Output: "(no cron jobs scheduled)"}, nil
	}
	jobs := make([]CronJob, 0, len(t.store.jobs))
	for _, j := range t.store.jobs {
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.Before(jobs[j].CreatedAt) })
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d active cron(s):\n\n", len(jobs))
	for _, j := range jobs {
		preview := j.Prompt
		if len(preview) > 60 {
			preview = preview[:57] + "..."
		}
		fmt.Fprintf(&sb, "  %s  [%s]  %q  (expires %s)\n", j.ID, j.Schedule, preview, j.ExpiresAt.Format("2006-01-02"))
	}
	return Result{Output: sb.String(), Metadata: map[string]any{"count": len(jobs)}}, nil
}
