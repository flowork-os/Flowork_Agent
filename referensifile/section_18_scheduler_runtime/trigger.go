// Package cron (tools) — manual trigger job by ID (bypass NextRun).

package cron

import (
	"context"
	"fmt"
	"strings"

	corecron "github.com/flowork/kernel/kernel/cron"
	"github.com/flowork/kernel/kernel/tools"
)

const ToolTriggerName = "cron.trigger"

func init() { tools.Register(&triggerTool{}) }

type triggerTool struct{}

func (t *triggerTool) Name() string { return ToolTriggerName }

func (t *triggerTool) Description() string {
	return "Manually trigger cron job by ID (bypass NextRun). Args: {id: string}. Returns: {id, result, error?}. Update LastRun + RunCount."
}

func (t *triggerTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
		},
		"required": []string{"id"},
	}
}

func (t *triggerTool) Run(ctx context.Context, args map[string]any) (any, error) {
	id, _ := args["id"].(string)
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("cron.trigger: id required")
	}

	job, err := corecron.GetJob(id)
	if err != nil {
		return nil, err
	}

	result, runErr := corecron.ExecuteJob(ctx, job)
	updated, _ := corecron.UpdateRun(job, runErr)

	out := map[string]any{
		"id":          updated.ID,
		"result":      result,
		"last_run":    updated.LastRun.Format("2006-01-02T15:04:05Z"),
		"run_count":   updated.RunCount,
	}
	if runErr != nil {
		out["error"] = runErr.Error()
	}
	return out, nil
}
