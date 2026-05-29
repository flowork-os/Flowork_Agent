// Package tools — skill_autocreate.go: Phase 2.1 Skill Auto-Creation Trigger.
//
// Hermes Agent UNIQUE pattern: agent auto-create skill .md file dari workflow
// yang berhasil 5+ tool call. Procedural memory yang self-improving.
//
// Pattern:
// 1. Track tool call counter per session
// 2. Saat session complete dengan 5+ tool call success, propose skillify
// 3. User confirm → invoke SkillTool dengan name='skillify' + workflow context
// 4. Skill .md dropped ke ~/.flowork/skills/auto/<name>.md
// 5. Next similar task → skill loaded instant

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
)

// sessionToolCalls — track tool call counter + outcome per session.
type sessionToolCalls struct {
	SessionID    string
	Calls        []string  // tool names invoked
	SuccessCount int
	StartedAt    time.Time
}

var (
	autoCreateMu       sync.RWMutex
	autoCreateSessions = map[string]*sessionToolCalls{}
)

// AutoCreateThreshold — min tool call success sebelum propose skill_create.
const AutoCreateThreshold = 5

// TrackToolCall — record tool call success per session. Caller (kernel
// dispatch) panggil ini post tool execution.
func TrackToolCall(sessionID, toolName string, success bool) {
	autoCreateMu.Lock()
	defer autoCreateMu.Unlock()
	s, ok := autoCreateSessions[sessionID]
	if !ok {
		s = &sessionToolCalls{
			SessionID: sessionID,
			StartedAt: time.Now(),
		}
		autoCreateSessions[sessionID] = s
	}
	s.Calls = append(s.Calls, toolName)
	if success {
		s.SuccessCount++
	}
}

// ShouldProposeSkill — return true kalau session udah meet AutoCreateThreshold.
func ShouldProposeSkill(sessionID string) bool {
	autoCreateMu.RLock()
	defer autoCreateMu.RUnlock()
	s, ok := autoCreateSessions[sessionID]
	if !ok {
		return false
	}
	return s.SuccessCount >= AutoCreateThreshold
}

// GetSessionTrace — return list tool name + counter for session (untuk skillify input).
func GetSessionTrace(sessionID string) ([]string, int) {
	autoCreateMu.RLock()
	defer autoCreateMu.RUnlock()
	s, ok := autoCreateSessions[sessionID]
	if !ok {
		return nil, 0
	}
	return s.Calls, s.SuccessCount
}

// ResetSession — clear tracking (setelah skill di-create atau session done).
func ResetSession(sessionID string) {
	autoCreateMu.Lock()
	defer autoCreateMu.Unlock()
	delete(autoCreateSessions, sessionID)
}

// SkillProposeTool — manual trigger skill propose dengan session trace.
type SkillProposeTool struct{}

type skillProposeArgs struct {
	SkillName   string `json:"skill_name" validate:"required"`
	Description string `json:"description" validate:"required"`
	SessionID   string `json:"session_id,omitempty"`
}

func NewSkillProposeTool() *SkillProposeTool { return &SkillProposeTool{} }

func (t *SkillProposeTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "SkillPropose",
		Description: "Propose new skill .md file dari recent workflow. Skill auto-creation trigger " +
			"saat session udah 5+ tool call success (Hermes UNIQUE self-improving pattern).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name":  map[string]any{"type": "string", "description": "kebab-case skill name"},
				"description": map[string]any{"type": "string", "description": "one-line skill description"},
				"session_id":  map[string]any{"type": "string", "description": "optional session ID untuk pull trace"},
			},
			"required": []string{"skill_name", "description"},
		},
	}
}

func (t *SkillProposeTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args skillProposeArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("SkillPropose: decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("SkillPropose: validation: %w", err)
	}

	sessionID := args.SessionID
	if sessionID == "" {
		sessionID = invocation.SessionID
	}
	trace, count := GetSessionTrace(sessionID)

	return Result{
		Output: fmt.Sprintf(`# Skill Propose

Name: %s
Description: %s

Recent session trace (%d tool calls, %d success):
%v

Next steps:
1. Invoke Skill(name='skillify') dengan workflow context untuk generate .md
2. Save ke ~/.flowork/skills/auto/%s.md
3. Loader Refresh() → instant available
`,
			args.SkillName, args.Description, len(trace), count, trace, args.SkillName),
		Metadata: map[string]any{
			"skill_name":    args.SkillName,
			"description":   args.Description,
			"trace_count":   len(trace),
			"success_count": count,
		},
	}, nil
}
