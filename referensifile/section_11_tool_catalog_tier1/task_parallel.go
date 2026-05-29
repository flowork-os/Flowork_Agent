package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/agents"
	"github.com/teetah2402/flowork/internal/provider"
)

// TaskParallelTool spawns multiple sub-agents concurrently.
type TaskParallelTool struct {
	runner SubAgentRunner
}

type taskParallelArgs struct {
	Tasks []taskParallelItem `json:"tasks"`
}

type taskParallelItem struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt" validate:"required"`
	SubagentType string `json:"subagent_type,omitempty"`
}

func NewTaskParallelTool(runner SubAgentRunner) *TaskParallelTool {
	return &TaskParallelTool{runner: runner}
}

func (t *TaskParallelTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "task_parallel",
		Description: "Spawn multiple sub-agents in parallel. Each runs independently with its own conversation. Returns all results when every sub-agent completes. Use for parallel research, investigation, or divide-and-conquer tasks.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tasks": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"description": map[string]any{
								"type":        "string",
								"description": "Short description of this sub-task.",
							},
							"prompt": map[string]any{
								"type":        "string",
								"description": "Self-contained task prompt. Include all necessary context.",
							},
							"subagent_type": map[string]any{
								"type":        "string",
								"description": "Optional: general | explorer | plan | verification. Default: general.",
							},
						},
						"required": []string{"description", "prompt"},
					},
					"description": "Array of sub-tasks to run in parallel.",
				},
			},
			"required": []string{"tasks"},
		},
	}
}

func (t *TaskParallelTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args taskParallelArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode task_parallel arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if len(args.Tasks) == 0 {
		return Result{}, fmt.Errorf("at least one task is required")
	}
	if t.runner == nil {
		return Result{}, fmt.Errorf("task runner not configured")
	}

	// Convert to agents.SubTask
	subTasks := make([]agents.SubTask, len(args.Tasks))
	for i, task := range args.Tasks {
		if strings.TrimSpace(task.Prompt) == "" {
			return Result{}, fmt.Errorf("task %d: prompt is required", i)
		}
		subTasks[i] = agents.SubTask{
			Description:  task.Description,
			Prompt:       task.Prompt,
			SubagentType: task.SubagentType,
			MaxSteps:     12,
		}
	}

	// Run all in parallel
	results := agents.RunSubTasksParallel(ctx, t.runner, subTasks)

	// Count successes/failures
	successes := 0
	failures := 0
	for _, r := range results {
		if r.Error != nil {
			failures++
		} else {
			successes++
		}
	}

	output := agents.FormatParallelResults(results)

	return Result{
		Output: output,
		Metadata: map[string]any{
			"total":     len(results),
			"successes": successes,
			"failures":  failures,
		},
	}, nil
}
