package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/teetah2402/flowork/internal/provider"
)

// GoalDoneTool — sentinel the autonomous loop watches for. The agent calls
// this when it believes the owner's goal has been achieved; the runner in
// internal/autonomous then breaks the never-stop loop instead of prompting
// it to continue. Calling this tool is cheap on purpose — we want the agent
// to volunteer completion eagerly rather than spinning on make-work.
//
// Why not just stop when the model emits a plain text reply? Because the
// autonomous runner re-prompts on plain replies ("continue working"). We
// need an explicit signal the agent has actively declared done; otherwise
// we can't distinguish "asking a clarifying question" from "finished".
type GoalDoneTool struct{}

type goalDoneArgs struct {
	Summary string `json:"summary" validate:"required"`
}

func NewGoalDoneTool() *GoalDoneTool { return &GoalDoneTool{} }

func (t *GoalDoneTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "goal_done",
		Description: "Signal that the owner's autonomous goal is fully achieved. Only call when the work is really finished, verified, and committed — the autonomous loop stops immediately after this.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "One-paragraph recap of what was done and how it was verified.",
				},
			},
			"required": []string{"summary"},
		},
	}
}

func (t *GoalDoneTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args goalDoneArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("goal_done: invalid arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	return Result{
		ToolName: "goal_done",
		OK:       true,
		Output:   "goal_done acknowledged: " + args.Summary,
		Metadata: map[string]any{"goal_done": true, "summary": args.Summary},
	}, nil
}
