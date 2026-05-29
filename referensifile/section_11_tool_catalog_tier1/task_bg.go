package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/sandbox"
)

// Background task registry — separate from TaskTool (which spawns sub-agents).
// TaskCreate starts a long-running bash command; TaskGet/List/Stop/Output manage them.

type BgTask struct {
	ID         string    `json:"id"`
	Command    string    `json:"command"`
	Status     string    `json:"status"` // running | completed | failed | stopped
	StartedAt  time.Time `json:"started_at" validate:"required"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ExitCode   int       `json:"exit_code" validate:"required"`
	Output     strings.Builder
	Cancel     context.CancelFunc
	mu         sync.Mutex
}

type BgTaskRegistry struct {
	mu    sync.Mutex
	tasks map[string]*BgTask
}

// Audit GAP #8 — long agent sessions can spawn thousands of background tasks,
// leaving their BgTask{Output: strings.Builder} records in memory forever.
// Cap the number of retained finished tasks to maxRetainedBgTasks; the oldest
// finished task is evicted once the cap is reached. Running tasks are never
// evicted (they still hold the cancel func and output buffer actively).
const maxRetainedBgTasks = 256

// BUG-H03 fix (2026-04-19) — cap output buffer per task ke 1 MiB. Command
// yang generate output raksasa (go test -v, build log, streaming) tidak
// lagi bisa OOM memori proses. Tail preserved (paling relevan untuk debug).
const maxTaskOutputBytes = 1 << 20 // 1 MiB

var globalBgTasks = &BgTaskRegistry{tasks: make(map[string]*BgTask)}

// gcFinishedTasks prunes the oldest finished/failed/stopped tasks when the
// map exceeds maxRetainedBgTasks. Called under r.mu held.
func (r *BgTaskRegistry) gcFinishedTasks() {
	if len(r.tasks) <= maxRetainedBgTasks {
		return
	}
	type kv struct {
		id  string
		end time.Time
	}
	var finished []kv
	for id, t := range r.tasks {
		t.mu.Lock()
		if !t.FinishedAt.IsZero() {
			finished = append(finished, kv{id, t.FinishedAt})
		}
		t.mu.Unlock()
	}
	if len(finished) == 0 {
		return
	}
	// Sort by end time ascending (oldest first).
	for i := 1; i < len(finished); i++ {
		for j := i; j > 0 && finished[j-1].end.After(finished[j].end); j-- {
			finished[j-1], finished[j] = finished[j], finished[j-1]
		}
	}
	// Evict until back under cap.
	for _, f := range finished {
		if len(r.tasks) <= maxRetainedBgTasks {
			return
		}
		delete(r.tasks, f.id)
	}
}

// ListBgTasksText returns a human-readable summary of all background tasks.
// Called by the /tasks slash command.
func ListBgTasksText() string {
	globalBgTasks.mu.Lock()
	defer globalBgTasks.mu.Unlock()
	if len(globalBgTasks.tasks) == 0 {
		return "(no background tasks)"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d background task(s):\n\n", len(globalBgTasks.tasks))
	for _, task := range globalBgTasks.tasks {
		task.mu.Lock()
		cmd := task.Command
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		line := fmt.Sprintf("  %-20s  %-10s  %q", task.ID, task.Status, cmd)
		if !task.FinishedAt.IsZero() {
			line += fmt.Sprintf("  (exit %d)", task.ExitCode)
		}
		fmt.Fprintln(&sb, line)
		task.mu.Unlock()
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (r *BgTaskRegistry) Start(cmd string) (*BgTask, error) {
	// BUG-H07 fix (2026-04-19): task_bg sebelumnya bypass ShellSafetyInterceptor.
	// Sekarang apply same blocklist yang dipakai bash tool — reject command
	// dengan fragment berbahaya (rm -rf /, mkfs, shutdown, fork bomb, dll).
	if err := checkTaskBgSafety(cmd); err != nil {
		return nil, err
	}

	id := fmt.Sprintf("bg-%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	task := &BgTask{
		ID:        id,
		Command:   cmd,
		Status:    "running",
		StartedAt: time.Now(),
		Cancel:    cancel,
	}
	r.mu.Lock()
	r.tasks[id] = task
	r.gcFinishedTasks()
	r.mu.Unlock()

	go func() {
		defer task.Cancel() // FIX: Context Memory Leak untuk zombie yang selesai natural
		defer func() {
			if r := recover(); r != nil {
				task.mu.Lock()
				task.FinishedAt = time.Now()
				task.Status = "failed"
				task.ExitCode = -1
				task.Output.WriteString(fmt.Sprintf("\nbg task panic: %v", r))
				task.mu.Unlock()
			}
		}()

		policy := sandbox.DefaultPolicy("")
		policy.Timeout = 24 * time.Hour
		policy.MaxOutputBytes = maxTaskOutputBytes

		res, err := sandbox.Run(ctx, policy, cmd)

		task.mu.Lock()
		task.FinishedAt = time.Now()

		var out string
		if res != nil {
			out = res.Stdout + res.Stderr
		}

		// BUG-H03 fix (2026-04-19): cap task.Output ke maxTaskOutputBytes
		if len(out) > maxTaskOutputBytes {
			truncated := len(out) - maxTaskOutputBytes
			task.Output.WriteString(fmt.Sprintf("...(truncated %d bytes at head to prevent OOM)...\n", truncated))
			out = out[truncated:]
		}
		task.Output.WriteString(out)

		if err != nil || (res != nil && res.ExitCode != 0) || (res != nil && res.Blocked != "") {
			if ctx.Err() == context.Canceled {
				task.Status = "stopped"
			} else {
				task.Status = "failed"
				if res != nil && res.Blocked != "" {
					task.Output.WriteString("\nBLOCKED BY SANDBOX: " + res.Blocked)
				}
			}
			if res != nil {
				task.ExitCode = res.ExitCode
			} else {
				task.ExitCode = -1
			}
		} else {
			task.Status = "completed"
			task.ExitCode = 0
		}
		task.mu.Unlock()
	}()
	return task, nil
}

// ─── task_create ───────────────────────────────────────────────────

type TaskCreateTool struct{}

func NewTaskCreateTool() *TaskCreateTool { return &TaskCreateTool{} }

type taskCreateArgs struct {
	Command string `json:"command" validate:"required"`
}

func (t *TaskCreateTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task_create",
		Description: "Start a long-running shell command as a background task. Returns task_id. Use task_get/task_output to poll.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required": []string{"command"},
		},
	}
}

func (t *TaskCreateTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args taskCreateArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	task, err := globalBgTasks.Start(args.Command)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Output:   fmt.Sprintf("Started background task %s", task.ID),
		Metadata: map[string]any{"id": task.ID, "status": "running"},
	}, nil
}

// ─── task_get ──────────────────────────────────────────────────────

type TaskGetTool struct{}

func NewTaskGetTool() *TaskGetTool { return &TaskGetTool{} }

type taskIDArg struct {
	ID string `json:"id" validate:"required"`
}

func (t *TaskGetTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task_get",
		Description: "Get status of a background task by ID.",
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"},
		},
	}
}

func (t *TaskGetTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args taskIDArg
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	globalBgTasks.mu.Lock()
	task, ok := globalBgTasks.tasks[args.ID]
	globalBgTasks.mu.Unlock()
	if !ok {
		return Result{}, fmt.Errorf("task %q not found", args.ID)
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	out := fmt.Sprintf("Task %s: %s (started %s)", task.ID, task.Status, task.StartedAt.Format(time.RFC3339))
	if !task.FinishedAt.IsZero() {
		out += fmt.Sprintf(", finished %s, exit=%d", task.FinishedAt.Format(time.RFC3339), task.ExitCode)
	}
	return Result{
		Output: out,
		Metadata: map[string]any{
			"id": task.ID, "status": task.Status, "exit_code": task.ExitCode,
		},
	}, nil
}

// ─── task_list ─────────────────────────────────────────────────────

type TaskListTool struct{}

func NewTaskListTool() *TaskListTool { return &TaskListTool{} }

func (t *TaskListTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task_list",
		Description: "List all background tasks (running + finished).",
		InputSchema: map[string]any{"type": "object"},
	}
}

func (t *TaskListTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	globalBgTasks.mu.Lock()
	defer globalBgTasks.mu.Unlock()
	if len(globalBgTasks.tasks) == 0 {
		return Result{Output: "(no tasks)"}, nil
	}
	var sb strings.Builder
	for id, task := range globalBgTasks.tasks {
		task.mu.Lock()
		cmd := task.Command
		if len(cmd) > 50 {
			cmd = cmd[:47] + "..."
		}
		fmt.Fprintf(&sb, "  %s  %-10s  %q\n", id, task.Status, cmd)
		task.mu.Unlock()
	}
	return Result{Output: sb.String(), Metadata: map[string]any{"count": len(globalBgTasks.tasks)}}, nil
}

// ─── task_stop ─────────────────────────────────────────────────────

type TaskStopTool struct{}

func NewTaskStopTool() *TaskStopTool { return &TaskStopTool{} }

func (t *TaskStopTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task_stop",
		Description: "Cancel a running background task.",
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"},
		},
	}
}

func (t *TaskStopTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args taskIDArg
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	globalBgTasks.mu.Lock()
	task, ok := globalBgTasks.tasks[args.ID]
	globalBgTasks.mu.Unlock()
	if !ok {
		return Result{}, fmt.Errorf("task %q not found", args.ID)
	}
	if task.Cancel != nil {
		task.Cancel()
	}
	return Result{Output: "Stopped task " + args.ID}, nil
}

// ─── task_output ───────────────────────────────────────────────────

type TaskOutputTool struct{}

func NewTaskOutputTool() *TaskOutputTool { return &TaskOutputTool{} }

func (t *TaskOutputTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task_output",
		Description: "Get accumulated stdout/stderr of a background task.",
		InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"},
		},
	}
}

func (t *TaskOutputTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args taskIDArg
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	globalBgTasks.mu.Lock()
	task, ok := globalBgTasks.tasks[args.ID]
	globalBgTasks.mu.Unlock()
	if !ok {
		return Result{}, fmt.Errorf("task %q not found", args.ID)
	}
	task.mu.Lock()
	defer task.mu.Unlock()
	raw := task.Output.String()
	// UTF-8 safe tail (prevents mid-codepoint slicing on emoji/non-Latin output).
	out := TailRunes(raw, 10000)
	if out != raw {
		out = "(truncated — showing last 10k runes)\n" + out
	}
	return Result{
		Output:   out,
		Metadata: map[string]any{"id": task.ID, "status": task.Status},
	}, nil
}

// checkTaskBgSafety — BUG-H07 fix. Mirrors ShellSafetyInterceptor disallow
// list (rm -rf /, mkfs, shutdown, fork bomb, etc.) plus sensitive file
// access patterns. Applied sebelum Start() execute command sehingga agent
// yang bypass tool "bash" via "task_create" tetap harus respect block list.
func checkTaskBgSafety(cmd string) error {
	low := strings.ToLower(cmd)
	disallowed := []string{
		"rm -rf /",
		"mkfs",
		"shutdown",
		"reboot",
		"poweroff",
		":(){:|:&};:",
	}
	for _, frag := range disallowed {
		if strings.Contains(low, frag) {
			return fmt.Errorf("task_create blocked by safety policy: %q", frag)
		}
	}
	// Reuse sensitive-file bash command detector (already exported).
	if isSensitiveBashCommand(cmd) {
		return errSensitive
	}
	return nil
}
