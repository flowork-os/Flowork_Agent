// Package tools — hooks_pretool.go: Phase 5.1 PreTool/PostTool hooks.
//
// Adopt Claude Code hooks pattern. Existing kernel/hooks/ punya PreChat +
// PostChat. Expand dengan PreTool + PostTool untuk per-tool extensibility:
//   - PreTool: validate args / inject context / block per policy
//   - PostTool: log result / update karma / track skill auto-create
//
// Hooks fire via internal registry. Tool dispatch panggil sebelum + sesudah
// Execute().

package tools

import (
	"context"
	"sync"
)

// ToolHookData — data passed ke hook handler.
type ToolHookData struct {
	WargaID    string
	SessionID  string
	ToolName   string
	Arguments  []byte
	Result     *Result
	Error      error
}

// ToolHookHandler — function signature handler. Return error untuk block
// execution (PreTool only).
type ToolHookHandler func(ctx context.Context, data *ToolHookData) error

type toolHookRegistry struct {
	mu          sync.RWMutex
	preHandlers  []ToolHookHandler
	postHandlers []ToolHookHandler
}

var globalToolHooks = &toolHookRegistry{}

// RegisterPreTool — register PreTool hook handler. Called sebelum tool execute.
// Return error untuk block.
func RegisterPreTool(h ToolHookHandler) {
	globalToolHooks.mu.Lock()
	defer globalToolHooks.mu.Unlock()
	globalToolHooks.preHandlers = append(globalToolHooks.preHandlers, h)
}

// RegisterPostTool — register PostTool hook handler. Called setelah tool execute.
func RegisterPostTool(h ToolHookHandler) {
	globalToolHooks.mu.Lock()
	defer globalToolHooks.mu.Unlock()
	globalToolHooks.postHandlers = append(globalToolHooks.postHandlers, h)
}

// FirePreTool — invoke all PreTool handlers. Return error pertama yang block.
func FirePreTool(ctx context.Context, data *ToolHookData) error {
	globalToolHooks.mu.RLock()
	handlers := append([]ToolHookHandler(nil), globalToolHooks.preHandlers...)
	globalToolHooks.mu.RUnlock()
	for _, h := range handlers {
		if err := h(ctx, data); err != nil {
			return err
		}
	}
	return nil
}

// FirePostTool — invoke all PostTool handlers (best-effort, errors logged but not blocking).
func FirePostTool(ctx context.Context, data *ToolHookData) {
	globalToolHooks.mu.RLock()
	handlers := append([]ToolHookHandler(nil), globalToolHooks.postHandlers...)
	globalToolHooks.mu.RUnlock()
	for _, h := range handlers {
		_ = h(ctx, data)
	}
}

// InitDefaultToolHooks — register default hook handlers di startup.
//
// Default hooks:
//   - PreTool: Plan mode enforcement (block destructive tool kalau IsPlanMode)
//   - PostTool: SkillAutoCreate tracker (count tool success per session)
func InitDefaultToolHooks() {
	// PreTool: Plan mode enforcement (Phase 1.5)
	// Existing global state via permissions.go (CurrentPermissionMode).
	RegisterPreTool(func(ctx context.Context, data *ToolHookData) error {
		if CurrentPermissionMode() != PermissionPlan {
			return nil
		}
		// Block destructive tools saat plan mode active
		destructive := map[string]bool{
			"write": true, "edit": true, "multiedit": true,
			"bash": true, "powershell": true,
			"notebookedit": true,
			"brain_post_drawer": true, "memorize_brain": true,
			"forum_post": true, "dream_post": true,
		}
		if destructive[data.ToolName] {
			return fmtErrPlanModeBlock(data.ToolName)
		}
		return nil
	})

	// PostTool: SkillAutoCreate tracker (Phase 2.1)
	RegisterPostTool(func(ctx context.Context, data *ToolHookData) error {
		success := data.Error == nil
		TrackToolCall(data.SessionID, data.ToolName, success)
		return nil
	})
}

func fmtErrPlanModeBlock(toolName string) error {
	return &PlanModeBlockError{ToolName: toolName}
}

// PlanModeBlockError — error type untuk Plan mode block.
type PlanModeBlockError struct {
	ToolName string
}

func (e *PlanModeBlockError) Error() string {
	return "tool '" + e.ToolName + "' blocked by Plan mode (read-only). Call ExitPlanMode dulu untuk execute."
}
