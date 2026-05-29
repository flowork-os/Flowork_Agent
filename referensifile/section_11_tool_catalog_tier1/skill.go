package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/teetah2402/flowork/internal/provider"
)

// Skill — a reusable instruction template that expands into extra system
// context when invoked. Built-in skills are registered at startup.
// User can also drop SKILL.md files in ~/.flowork/skills/<name>/SKILL.md.
type Skill struct {
	Name        string
	Description string
	Prompt      string
}

// SkillRegistry — central store for available skills.
//
// userSources tracks the resolved scope label per user-defined skill so we
// can emit a conflict warning when the same skill name appears in 2+ scopes
// (effekdomino #56 — silent last-write-wins overwrite).
type SkillRegistry struct {
	skills      map[string]Skill
	userSources map[string]string
}

func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{
		skills:      make(map[string]Skill),
		userSources: make(map[string]string),
	}
}

func (r *SkillRegistry) Register(s Skill) {
	r.skills[strings.ToLower(s.Name)] = s
}

// RegisterUserSkill registers a skill loaded from a SKILL.md file and
// records the scope label (global/workspace-private/committed). When the
// same name was already registered from a different scope, a warning is
// logged so operators can spot silent overrides instead of debugging
// ghosted skill content.
//
// Caller order encodes precedence: load global first, then committed, then
// workspace-private — last write wins, so workspace-private has the highest
// priority. Match effekdomino #56 fix path #2.
func (r *SkillRegistry) RegisterUserSkill(s Skill, scope string) {
	key := strings.ToLower(s.Name)
	if prev, ok := r.userSources[key]; ok && prev != scope {
		log.Printf("[skill] conflict: %q already loaded from %s, overridden by %s",
			key, prev, scope)
	}
	r.skills[key] = s
	r.userSources[key] = scope
}

// UserSkillSource returns the scope label of a user-defined skill (or empty
// when the skill is built-in / unknown). Used by the skills_api endpoint to
// expose source path per skill (effekdomino #56 fix path #3).
func (r *SkillRegistry) UserSkillSource(name string) string {
	return r.userSources[strings.ToLower(name)]
}

func (r *SkillRegistry) Get(name string) (Skill, bool) {
	s, ok := r.skills[strings.ToLower(name)]
	return s, ok
}

func (r *SkillRegistry) Names() []string {
	n := make([]string, 0, len(r.skills))
	for k := range r.skills {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// DefaultSkills — 5 essential skills ported from Claude Code.
func DefaultSkills() *SkillRegistry {
	r := NewSkillRegistry()
	r.Register(Skill{
		Name:        "remember",
		Description: "Save user preference / context for future sessions to ~/.flowork/memory/",
		Prompt: `Use the 'write' tool to append a note to ~/.flowork/memory/notes.md.
Format: timestamp + topic + content. Keep each note under 3 lines.
Before writing, check if similar note exists via 'grep' — update instead of duplicate.
After writing, confirm briefly with one sentence summary.`,
	})
	r.Register(Skill{
		Name:        "debug",
		Description: "Systematically investigate a bug: reproduce, isolate, hypothesize, verify",
		Prompt: `Follow this structured debug flow:
1. REPRODUCE — confirm the bug happens consistently. Use 'bash' / 'read' to see current behavior.
2. ISOLATE — narrow to smallest failing case. Check recent changes via git log if relevant.
3. HYPOTHESIZE — state 1-3 plausible causes ranked by likelihood.
4. VERIFY — test each hypothesis with minimal change (read code, add print, run).
5. FIX — apply minimal fix. Explain why it solves the root cause.
6. REGRESS — if possible, add a test preventing recurrence.
Do NOT guess-fix. Do NOT refactor surrounding code unless it IS the cause.`,
	})
	r.Register(Skill{
		Name:        "verify",
		Description: "Double-check a claim or implementation before committing to it",
		Prompt: `Before stating something as true, VERIFY it:
- Read file contents instead of assuming.
- Run the command instead of predicting output.
- Check git log instead of guessing history.
- Test the function instead of believing the name.
Then state the claim with evidence: "I verified X by doing Y, result: Z."
If verification fails or is impossible, say so explicitly.`,
	})
	r.Register(Skill{
		Name:        "simplify",
		Description: "Review code for unnecessary complexity and remove it",
		Prompt: `Review the target code for:
- Dead code (unused imports, functions, variables)
- Premature abstraction (interface with 1 implementer, factory for trivial constructors)
- Redundant safety (null checks on already-validated input, try-catch that just rethrows)
- Over-engineering (config flags for untoggleable behavior, hooks for single caller)
- Duplicate logic (similar helpers in different places)

Propose removals/consolidations. Apply only safe ones (those confirmed unused by grep).
Do NOT break interfaces or introduce hidden behavior changes.`,
	})
	r.Register(Skill{
		Name:        "batch",
		Description: "Run multiple related tasks in parallel when independent",
		Prompt: `When the user asks for N tasks that are independent (no shared state, no ordering),
run them in parallel. Use the 'task' tool with separate sub-agents, OR issue multiple
tool calls in a single response when the underlying tools are side-effect-free (read, grep, glob).

After all tasks complete, consolidate results into one concise summary.
If any task fails, report which one + why, then continue with the rest.`,
	})

	// ─── Phase 05: 10 more skills ──────────────────────────────────
	r.Register(Skill{
		Name:        "update-config",
		Description: "Modify ~/.flowork/config.yaml safely (read current, propose change, apply via config tool)",
		Prompt: `Use the 'config' tool to list current config, then propose exact key+value change.
Ask user to confirm via brief tool if not obvious. Apply via config action=set.
Never edit the file directly via write/edit — always via config tool.`,
	})
	r.Register(Skill{
		Name:        "keybindings",
		Description: "Customize ~/.flowork/keybindings.json (not implemented runtime yet)",
		Prompt:      `Keybindings are stored at ~/.flowork/keybindings.json. Read current, merge user's request, write back. Note: runtime hot-reload not yet wired — user must restart flowork.`,
	})
	r.Register(Skill{
		Name:        "loop",
		Description: "Schedule a recurring prompt via cron_create tool",
		Prompt: `Use cron_create tool with 5-field cron schedule + user's prompt.
Default schedule: '*/5 * * * *' (every 5 min). User can override.
Reminder: cron auto-expires after 7 days.`,
	})
	r.Register(Skill{
		Name:        "schedule",
		Description: "Manage remote/cloud-hosted agents (stub — not yet wired to backend)",
		Prompt:      `Remote scheduling not yet active in FLOWORK Go. For now, use cron_create for local scheduling.`,
	})
	r.Register(Skill{
		Name:        "stuck",
		Description: "Offer 5 alternative approaches when current path isn't working",
		Prompt: `The user (or you) feel stuck. List 5 alternative approaches to the CURRENT blocker:
1. **Reverse** — Assume opposite of current assumption
2. **Simplify** — Strip constraints, solve easiest form
3. **Tools** — Which existing tool haven't we tried?
4. **Reference** — Search docs / websearch for similar problem
5. **Delegate** — Spawn task sub-agent with explorer type

Rank by likely-to-work. Recommend #1.`,
	})
	r.Register(Skill{
		Name:        "lorem-ipsum",
		Description: "Generate placeholder text (150-500 words) for UI mockups",
		Prompt: `Generate lorem ipsum placeholder text. Default 100 words. User can specify length.
Return plain text, no markdown wrapper. No extra commentary.`,
	})
	r.Register(Skill{
		Name:        "skillify",
		Description: "Convert current conversation pattern into reusable skill",
		Prompt: `Analyze recent conversation. Extract:
1. Trigger — when would user invoke this?
2. Steps — what tools/actions did we do?
3. Output — what's the final deliverable?

Write as new SKILL.md in ~/.flowork/skills/<name>/SKILL.md using frontmatter:
---
name: skill-name
description: one line
---

# Instructions
...steps...`,
	})
	r.Register(Skill{
		Name:        "flow-in-chrome",
		Description: "Integrate FLOWORK into Chrome extension (deferred)",
		Prompt:      `Chrome extension integration not yet available. Use browser_* tools for current browser automation needs.`,
	})
	r.Register(Skill{
		Name:        "flow-api",
		Description: "Guide on building apps against FLOWORK Go SDK",
		Prompt: `FLOWORK Go SDK: import 'github.com/teetah2402/flowork/internal/core' + 'internal/provider' + 'internal/tools'.
Build custom CLI: NewAgent + NewSession + RunTurn.
See cmd/testagent/main.go for minimal example.`,
	})
	r.Register(Skill{
		Name:        "claude-api",
		Description: "Guide on using Anthropic Claude API directly with prompt caching",
		Prompt: `Anthropic Claude API via 'github.com/anthropics/anthropic-sdk-go':
- Set cache_control: {type: ephemeral} on system + tools
- Header: anthropic-beta: prompt-caching-2024-07-31
- Parse cache_creation_input_tokens + cache_read_input_tokens in response
- FLOWORK adapter at internal/provider/anthropic.go already implements this.`,
	})
	r.Register(Skill{
		Name:        "dream",
		Description: "Long-horizon planning (multi-day / multi-session)",
		Prompt: `Break user's goal into phases spanning multiple sessions:
1. Current session scope (today)
2. Next session prep (what to remember)
3. Final goal (end state)

Save phases to ~/.flowork/memory/project/<hash>.md via write tool so future sessions auto-load.`,
	})
	r.Register(Skill{
		Name:        "hunter",
		Description: "Automated code review — find bugs, bad patterns, security issues",
		Prompt: `Review target code for:
1. Security: injection, auth bypass, hardcoded secrets
2. Reliability: nil deref, error ignore, race conditions
3. Performance: O(n^2) loops, unnecessary allocations
4. Style: naming violations, dead code, overly complex

Spawn task tool with subagent_type=verification for each file. Consolidate findings.`,
	})
	r.Register(Skill{
		Name:        "run-skill-generator",
		Description: "Auto-generate new skill based on observed patterns",
		Prompt: `Scan recent conversation for repeated patterns. If found:
1. Propose skill name + description
2. Draft SKILL.md
3. Confirm with user via askuserquestion
4. Write via write tool to ~/.flowork/skills/<name>/SKILL.md`,
	})

	// User-defined skills are auto-loaded by DefaultRegistry via the
	// (*SkillRegistry).LoadUserSkills method in skill_markdown.go. That
	// path uses mdloader which strips UTF-8 BOM and also scans workspace
	// skills, whereas the earlier package-level loader removed on
	// 2026-04-17 silently dropped frontmatter on any BOM-prefixed file
	// (Kategori 5 parser fault) and registered the same skill twice
	// under inconsistent keys.
	return r
}

// SkillTool — invoke a skill by name.
type SkillTool struct {
	registry *SkillRegistry
}

type skillArgs struct {
	Skill string `json:"skill" validate:"required"`
	Args  string `json:"args,omitempty"`
}

func NewSkillTool(registry *SkillRegistry) *SkillTool {
	return &SkillTool{registry: registry}
}

func (t *SkillTool) Definition() provider.ToolDefinition {
	names := t.registry.Names()
	desc := "Invoke a built-in skill (pre-defined workflow). Available: " + strings.Join(names, ", ")
	return provider.ToolDefinition{
		Name:        "skill",
		Description: desc + ". Returns the skill's instructions as extra context for you to follow on next turn.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill": map[string]any{
					"type":        "string",
					"description": "Skill name (e.g. debug, verify, remember)",
				},
				"args": map[string]any{
					"type":        "string",
					"description": "Optional args/context passed to skill",
				},
			},
			"required": []string{"skill"},
		},
	}
}

func (t *SkillTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args skillArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode skill arguments: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	skill, ok := t.registry.Get(args.Skill)
	if !ok {
		avail := strings.Join(t.registry.Names(), ", ")
		return Result{}, fmt.Errorf("unknown skill %q (available: %s)", args.Skill, avail)
	}
	out := fmt.Sprintf("# Skill: %s\n\n%s\n\n%s", skill.Name, skill.Description, skill.Prompt)
	if args.Args != "" {
		out += "\n\n# Args from caller:\n" + args.Args
	}
	return Result{
		Output: out,
		Metadata: map[string]any{
			"skill": skill.Name,
		},
	}, nil
}
