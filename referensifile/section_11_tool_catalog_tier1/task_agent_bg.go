package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/agents"
	"github.com/teetah2402/flowork/internal/provider"
)

// Background sub-agent tasks — like task_bg.go for shell commands but for
// fresh sub-agents spawned via TaskTool with background=true. Parent gets a
// task ID immediately and can later call agent_task_get / agent_task_list.
//
// This is the lightweight Claude-Code-style fork: parent does NOT block on
// the sub-agent. The sub-agent runs to completion in its own goroutine,
// stores output, and is reaped on demand. No shared session — fork inherits
// only the prompt the parent supplied (matches existing TaskTool semantics).

type BgAgentTask struct {
	ID         string    `json:"id"`
	Type       string    `json:"agent_type"`
	Desc       string    `json:"description"`
	Status     string    `json:"status"` // running | completed | failed | stopped
	StartedAt  time.Time `json:"started_at" validate:"required"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	cancel     context.CancelFunc
	mu         sync.Mutex
}

type BgAgentRegistry struct {
	mu    sync.Mutex
	tasks map[string]*BgAgentTask
}

// Opus batch-1 audit fix (Kategori 3 — resource leak): mirror the
// retention cap already in place for task_bg.go. Long agent sessions
// can spawn hundreds of background sub-agents; previously every
// BgAgentTask (including its full Output string) stayed in the map
// forever. Cap retained finished tasks; running tasks are never evicted.
const maxRetainedBgAgents = 256

var globalBgAgents = &BgAgentRegistry{tasks: make(map[string]*BgAgentTask)}

// gcFinishedAgents prunes the oldest finished/failed/stopped entries
// when the map exceeds maxRetainedBgAgents. Caller must hold r.mu.
func (r *BgAgentRegistry) gcFinishedAgents() {
	if len(r.tasks) <= maxRetainedBgAgents {
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
	for _, f := range finished {
		if len(r.tasks) <= maxRetainedBgAgents {
			return
		}
		delete(r.tasks, f.id)
	}
}

// maxBgAgentDuration caps a single background sub-agent's wall-clock. The
// foreground agent loop already has a 30-min ceiling in core/agent.go, but
// bg agents previously got context.Background() and could outlive it —
// EXTBUG-008: stuck tool-call loops drained API credits indefinitely.
const maxBgAgentDuration = 15 * time.Minute

func (r *BgAgentRegistry) start(runner SubAgentRunner, agentType agents.AgentType, prompt, desc string) *BgAgentTask {
	id := fmt.Sprintf("bgagent-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), maxBgAgentDuration)
	t := &BgAgentTask{
		ID:        id,
		Type:      agentType.Name,
		Desc:      desc,
		Status:    "running",
		StartedAt: time.Now(),
		cancel:    cancel,
	}
	r.mu.Lock()
	r.tasks[id] = t
	r.gcFinishedAgents()
	r.mu.Unlock()

	go func() {
		defer t.cancel() // FIX: Mencegah Context Leak
		defer func() {
			if r := recover(); r != nil {
				t.mu.Lock()
				t.FinishedAt = time.Now()
				t.Status = "failed"
				t.Error = fmt.Sprintf("bg agent panic: %v", r)
				t.mu.Unlock()
			}
		}()
		var (
			out string
			err error
		)
		// Gemini audit fix (Bug 8.1): previously the non-typed fallback
		// (RunSubTask) silently bypassed per-type tool whitelists, so an
		// explorer/plan/verification sub-agent retained full tool access.
		// Now we refuse the fallback when the agent type declares tool
		// restrictions — better to fail loudly than leak capabilities.
		if typed, ok := runner.(TypedSubAgentRunner); ok {
			out, err = typed.RunTypedSubTask(ctx, agentType, prompt, 12)
		} else if len(agentType.AllowedTools) == 0 && len(agentType.ForbiddenTools) == 0 {
			// No tool restrictions declared → fallback is safe.
			out, err = runner.RunSubTask(ctx, agentType.SystemPrompt, prompt, 12)
		} else {
			err = fmt.Errorf(
				"sub-agent %q declares tool restrictions (allowed=%v forbidden=%v) but launcher is not a TypedSubAgentRunner; refusing to spawn with full permissions",
				agentType.Name, agentType.AllowedTools, agentType.ForbiddenTools)
		}
		t.mu.Lock()
		t.FinishedAt = time.Now()
		if err != nil {
			if ctx.Err() == context.Canceled {
				t.Status = "stopped"
			} else {
				t.Status = "failed"
			}
			t.Error = err.Error()
		} else {
			t.Status = "completed"
			t.Output = out
		}
		t.mu.Unlock()
	}()
	return t
}

func (r *BgAgentRegistry) get(id string) *BgAgentTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tasks[id]
}

func (r *BgAgentRegistry) list() []*BgAgentTask {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*BgAgentTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		out = append(out, t)
	}
	return out
}

func (t *BgAgentTask) snapshot() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := map[string]any{
		"id":          t.ID,
		"agent_type":  t.Type,
		"description": t.Desc,
		"status":      t.Status,
		"started_at":  t.StartedAt.Format(time.RFC3339),
	}
	if !t.FinishedAt.IsZero() {
		m["finished_at"] = t.FinishedAt.Format(time.RFC3339)
		m["duration"] = t.FinishedAt.Sub(t.StartedAt).Round(time.Millisecond).String()
	}
	if t.Output != "" {
		m["output"] = t.Output
	}
	if t.Error != "" {
		m["error"] = t.Error
	}
	return m
}

// ─── agent_task_get ──────────────────────────────────────────────────────

type AgentTaskGetTool struct{}

func NewAgentTaskGetTool() *AgentTaskGetTool { return &AgentTaskGetTool{} }

type agentTaskGetArgs struct {
	ID string `json:"id" validate:"required"`
}

func (t *AgentTaskGetTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "agent_task_get",
		Description: "Ambil status & output dari sebuah background sub-agent task (yang dispawn lewat task tool dengan background=true). Status: running | completed | failed | stopped. Output dikembalikan saat status completed/failed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Task ID seperti bgagent-<ts>."},
			},
			"required": []string{"id"},
		},
	}
}

func (t *AgentTaskGetTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args agentTaskGetArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	task := globalBgAgents.get(strings.TrimSpace(args.ID))
	if task == nil {
		return Result{ToolName: "agent_task_get", OK: false, Output: "task not found: " + args.ID}, nil
	}
	snap := task.snapshot()
	b, _ := json.MarshalIndent(snap, "", "  ")
	return Result{ToolName: "agent_task_get", OK: true, Output: string(b), Metadata: snap}, nil
}

// ─── agent_task_output ───────────────────────────────────────────────────

// AgentTaskOutputTool — Claude-Code-style TaskOutput: kembalikan partial
// output dari bg sub-agent yang masih running. Untuk sub-agent (bukan bash
// bg), "partial output" = output terakhir kalau sudah selesai, atau indikasi
// "still running" jika belum. Tidak ada streaming internal karena sub-agent
// baru tulis Output saat selesai; tool ini deterministik: OK kalau selesai,
// "pending" kalau masih running.
type AgentTaskOutputTool struct{}

func NewAgentTaskOutputTool() *AgentTaskOutputTool { return &AgentTaskOutputTool{} }

type agentTaskOutputArgs struct {
	ID string `json:"id" validate:"required"`
}

func (t *AgentTaskOutputTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "agent_task_output",
		Description: "Ambil output bg sub-agent task. Kalau status running → return indikasi pending; kalau completed → return full output; kalau failed → return error. Mirip TaskOutput Claude Code tapi scope sub-agent.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string", "description": "Task ID bgagent-<ts>."},
			},
			"required": []string{"id"},
		},
	}
}

func (t *AgentTaskOutputTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args agentTaskOutputArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, err
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	task := globalBgAgents.get(strings.TrimSpace(args.ID))
	if task == nil {
		return Result{ToolName: "agent_task_output", OK: false, Output: "task not found: " + args.ID}, nil
	}
	task.mu.Lock()
	status := task.Status
	output := task.Output
	errMsg := task.Error
	task.mu.Unlock()

	switch status {
	case "running":
		return Result{ToolName: "agent_task_output", OK: true,
			Output:   "pending: task masih berjalan — coba poll lagi beberapa detik ke depan",
			Metadata: map[string]any{"status": "running"}}, nil
	case "completed":
		return Result{ToolName: "agent_task_output", OK: true, Output: output}, nil
	case "failed":
		return Result{ToolName: "agent_task_output", OK: false, Output: "failed: " + errMsg}, nil
	case "stopped":
		return Result{ToolName: "agent_task_output", OK: false, Output: "stopped by cancel"}, nil
	default:
		// no-op — exhaustive switch guard
	}
	return Result{ToolName: "agent_task_output", OK: false, Output: "unknown status: " + status}, nil
}

// ─── agent_task_list ─────────────────────────────────────────────────────

type AgentTaskListTool struct{}

func NewAgentTaskListTool() *AgentTaskListTool { return &AgentTaskListTool{} }

func (t *AgentTaskListTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "agent_task_list",
		Description: "List semua background sub-agent tasks (running + finished). Pakai untuk cek progress beberapa task paralel sekaligus.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (t *AgentTaskListTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	tasks := globalBgAgents.list()
	if len(tasks) == 0 {
		return Result{ToolName: "agent_task_list", OK: true, Output: "(no background sub-agent tasks)"}, nil
	}
	out := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.snapshot())
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return Result{ToolName: "agent_task_list", OK: true, Output: string(b)}, nil
}
