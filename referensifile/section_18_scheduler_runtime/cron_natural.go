// Package tools — cron_natural.go: Phase 4.2 Cron Natural Language Parser.
//
// Adopt Hermes Agent natural language cron pattern. Parse "tiap hari 8 pagi"
// → cron expression "0 8 * * *". Wrapper di atas existing cron_create.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// CronNaturalTool — parse natural language → cron expression.
type CronNaturalTool struct{}

type cronNaturalArgs struct {
	Description string `json:"description" validate:"required"` // "tiap hari 8 pagi cek bug bounty"
	TaskName    string `json:"task_name,omitempty"`
}

func NewCronNaturalTool() *CronNaturalTool { return &CronNaturalTool{} }

func (t *CronNaturalTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "CronNatural",
		Description: "Parse natural language schedule + task → cron entry. " +
			"Contoh: 'tiap hari 8 pagi cek bug bounty' → cron '0 8 * * *' + task. " +
			"Wraps cron_create tool.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{"type": "string", "description": "Natural language schedule + task"},
				"task_name":   map[string]any{"type": "string", "description": "Optional task name"},
			},
			"required": []string{"description"},
		},
	}
}

// parseCronNL — minimal heuristic parser (full impl gunakan LLM call kelak).
func parseCronNL(input string) (cronExpr, taskDesc string) {
	lower := strings.ToLower(input)
	cronExpr = "0 9 * * *" // default daily 9 AM

	// Frequency detection
	switch {
	case strings.Contains(lower, "tiap menit"), strings.Contains(lower, "every minute"):
		cronExpr = "* * * * *"
	case strings.Contains(lower, "tiap jam"), strings.Contains(lower, "hourly"):
		cronExpr = "0 * * * *"
	case strings.Contains(lower, "tiap hari"), strings.Contains(lower, "daily"):
		cronExpr = "0 9 * * *"
	case strings.Contains(lower, "weekly"), strings.Contains(lower, "tiap minggu"):
		cronExpr = "0 9 * * 1"
	case strings.Contains(lower, "monthly"), strings.Contains(lower, "tiap bulan"):
		cronExpr = "0 9 1 * *"
	}

	// Time of day detection
	for hour := 0; hour < 24; hour++ {
		marker := fmt.Sprintf(" %d ", hour)
		if strings.Contains(lower, marker) || strings.HasSuffix(lower, fmt.Sprintf(" %d", hour)) {
			fields := strings.Fields(cronExpr)
			if len(fields) >= 2 {
				fields[1] = fmt.Sprintf("%d", hour)
				cronExpr = strings.Join(fields, " ")
			}
			break
		}
	}

	// Task = everything after schedule keyword
	for _, marker := range []string{"tiap hari", "daily", "tiap jam", "hourly", "tiap minggu", "weekly", "tiap bulan", "monthly"} {
		if idx := strings.Index(lower, marker); idx != -1 {
			taskDesc = strings.TrimSpace(input[idx+len(marker):])
			break
		}
	}
	if taskDesc == "" {
		taskDesc = input
	}

	return cronExpr, taskDesc
}

func (t *CronNaturalTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args cronNaturalArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("CronNatural: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("CronNatural: validation: %w", err)
	}
	cronExpr, taskDesc := parseCronNL(args.Description)
	if args.TaskName == "" {
		args.TaskName = "auto-cron-task"
	}
	return Result{
		Output: fmt.Sprintf(`# CronNatural Parsed

Input: %s
Parsed cron: %s
Task: %s
Task name: %s

Next: invoke cron_create tool dengan schedule='%s' + task='%s'.
`, args.Description, cronExpr, taskDesc, args.TaskName, cronExpr, taskDesc),
		Metadata: map[string]any{
			"cron_expr": cronExpr,
			"task_desc": taskDesc,
			"task_name": args.TaskName,
		},
	}, nil
}
