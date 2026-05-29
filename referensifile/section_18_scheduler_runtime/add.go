// Package cron (tools) — wrap kernel/cron CRUD as tools.
//
// Tool naming convention: cron.<verb>. Args mirror kernel/cron API.

package cron

import (
	"context"
	"fmt"
	"strings"

	corecron "github.com/flowork/kernel/kernel/cron"
	"github.com/flowork/kernel/kernel/tools"
)

const ToolAddName = "cron.add"

func init() { tools.Register(&addTool{}) }

type addTool struct{}

func (t *addTool) Name() string { return ToolAddName }

func (t *addTool) Description() string {
	return "Add cron job. Args: {name, expression: '*/15 * * * *', action: {type: 'tool'|'warga', tool?, target?, args?}, enabled?: bool}. Returns: {id, next_run}."
}

func (t *addTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":       map[string]any{"type": "string"},
			"expression": map[string]any{"type": "string"},
			"action": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":   map[string]any{"type": "string", "enum": []string{"tool", "warga"}},
					"tool":   map[string]any{"type": "string"},
					"target": map[string]any{"type": "string"},
					"args":   map[string]any{"type": "object"},
				},
				"required": []string{"type"},
			},
			"enabled": map[string]any{"type": "boolean", "default": true},
		},
		"required": []string{"name", "expression", "action"},
	}
}

func (t *addTool) Run(ctx context.Context, args map[string]any) (any, error) {
	name, _ := args["name"].(string)
	expression, _ := args["expression"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(expression) == "" {
		return nil, fmt.Errorf("cron.add: name + expression required")
	}

	actionRaw, _ := args["action"].(map[string]any)
	if actionRaw == nil {
		return nil, fmt.Errorf("cron.add: action object required")
	}

	action := corecron.JobAction{}
	action.Type, _ = actionRaw["type"].(string)
	action.Tool, _ = actionRaw["tool"].(string)
	action.Target, _ = actionRaw["target"].(string)
	action.Args, _ = actionRaw["args"].(map[string]any)

	enabled := true
	if v, ok := args["enabled"].(bool); ok {
		enabled = v
	}

	job := corecron.Job{
		Name:       name,
		Expression: expression,
		Action:     action,
		Enabled:    enabled,
	}
	saved, err := corecron.SaveJob(job)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":       saved.ID,
		"next_run": saved.NextRun.Format("2006-01-02T15:04:05Z"),
	}, nil
}
