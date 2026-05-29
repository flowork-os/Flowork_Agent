// Package cron (tools) — remove job by ID.

package cron

import (
	"context"
	"fmt"
	"strings"

	corecron "github.com/flowork/kernel/kernel/cron"
	"github.com/flowork/kernel/kernel/tools"
)

const ToolRemoveName = "cron.remove"

func init() { tools.Register(&removeTool{}) }

type removeTool struct{}

func (t *removeTool) Name() string { return ToolRemoveName }

func (t *removeTool) Description() string {
	return "Remove cron job by ID (idempotent). Args: {id: string}. Returns: {id, removed: true}."
}

func (t *removeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string"},
		},
		"required": []string{"id"},
	}
}

func (t *removeTool) Run(ctx context.Context, args map[string]any) (any, error) {
	id, _ := args["id"].(string)
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("cron.remove: id required")
	}
	if err := corecron.DeleteJob(id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "removed": true}, nil
}
