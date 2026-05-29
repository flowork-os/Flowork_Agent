package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/agents"
	"github.com/teetah2402/flowork/internal/provider"
)

// SubAgentRunner — abstraction over core.Agent.RunTurn to avoid import cycle.
// Concrete implementation wired at main.go.
type SubAgentRunner interface {
	RunSubTask(ctx context.Context, systemPrompt, userInput string, maxSteps int) (string, error)
}

// TypedSubAgentRunner — optional upgrade: runs with tool filter per agent type.
// If runner implements this, TaskTool uses it for permission enforcement.
type TypedSubAgentRunner interface {
	RunTypedSubTask(ctx context.Context, agentType agents.AgentType, userInput string, maxSteps int) (string, error)
}

// TaskTool — delegate a self-contained task to a sub-agent.
// The sub-agent runs in a fresh conversation with its own system prompt,
// returns final text output. Use for parallelizable research, isolated
// refactors, or "send an explorer" style tasks.
type TaskTool struct {
	runner SubAgentRunner
}

type taskArgs struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt" validate:"required"`
	SubagentType string `json:"subagent_type,omitempty"` // "general" | "explorer" | "plan" | "verification" | "statusline"
	// Background (Claude-Code-style fork-lite): kalau true, sub-agent dispawn
	// di goroutine terpisah dan kembalikan task ID segera. Parent lanjut
	// kerja, lalu poll hasilnya via agent_task_get / agent_task_list.
	Background bool `json:"background,omitempty"`
}

func NewTaskTool(runner SubAgentRunner) *TaskTool {
	return &TaskTool{runner: runner}
}

func (t *TaskTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task",
		Description: "Delegasikan task self-contained ke fresh sub-agent. Gunakan untuk riset paralel, investigasi terisolasi, atau task yang tidak ingin mengotori conversation utama. Mengembalikan output text final sub-agent.\n\nTypes: general (default, full tools), explorer (read-only), plan (analysis-only), verification (read+test), statusline (settings only).\n\nMode: synchronous (default, parent menunggu) atau background:true (fork — parent terima task ID dan lanjut kerja; poll hasil via agent_task_get / agent_task_list). Pakai background untuk paralel banyak riset bersamaan.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "Short (3-5 word) description of the task.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "Self-contained task prompt. Sub-agent has no memory of main conversation — include all context.",
				},
				"subagent_type": map[string]any{
					"type":        "string",
					"description": "Agent type: general | explorer | plan | verification | statusline. Default: general.",
					"enum":        []string{"general", "explorer", "plan", "verification", "statusline"},
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "Kalau true, sub-agent jalan di background goroutine. Tool langsung return task ID; parent lanjut kerja, lalu poll hasil dengan agent_task_get/agent_task_list. Default: false (synchronous).",
				},
			},
			"required": []string{"description", "prompt"},
		},
	}
}

func (t *TaskTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args taskArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode task arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Prompt) == "" {
		return Result{}, fmt.Errorf("prompt is required")
	}
	if t.runner == nil {
		return Result{}, fmt.Errorf("task runner not configured")
	}

	// Resolve agent type for system prompt and restrictions
	agentType := agents.GetAgentType(args.SubagentType)

	// Background fork: spawn goroutine, return task ID immediately. Parent
	// can then continue working and poll via agent_task_get / agent_task_list.
	if args.Background {
		task := globalBgAgents.start(t.runner, agentType, args.Prompt, args.Description)
		return Result{
			ToolName: "task",
			OK:       true,
			Output: fmt.Sprintf("background sub-agent started: id=%s type=%s\nPoll status with agent_task_get { id: %q }",
				task.ID, agentType.Name, task.ID),
			Metadata: map[string]any{
				"task_id":       task.ID,
				"subagent_type": agentType.Name,
				"background":    true,
				"description":   args.Description,
			},
		}, nil
	}

	var output string
	var err error
	// Prefer typed runner for actual tool filtering; fall back to system-prompt-only.
	if typedRunner, ok := t.runner.(TypedSubAgentRunner); ok {
		output, err = typedRunner.RunTypedSubTask(ctx, agentType, args.Prompt, 12)
	} else {
		output, err = t.runner.RunSubTask(ctx, agentType.SystemPrompt, args.Prompt, 12)
	}
	if err != nil {
		return Result{}, fmt.Errorf("sub-agent: %w", err)
	}

	return Result{
		Output: output,
		Metadata: map[string]any{
			"description":   args.Description,
			"subagent_type": agentType.Name,
		},
	}, nil
}
