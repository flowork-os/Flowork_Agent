package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/sandbox"
)

// shaFormatRE is the accepted git commit SHA shape for git_rollback — 7..40
// lowercase hex. Rejects refspecs (origin/main), revisions (HEAD~5), and any
// dash-led argument injection (`--hard --`). EXTBUG-004.
var shaFormatRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// Git-safety toolset for owner-mode autonomous self-modification.
//
// Workflow the system prompt tells the agent to follow:
//   1. git_checkpoint { message } — commits current state + pushes to remote
//      on a fresh branch autonomous/<ts>. Ensures there is always a
//      rollback point that survives local disk loss.
//   2. edit files via the usual Edit / Write / MultiEdit tools
//   3. git_verify — runs `go build ./...` + `go test ./...`. Must pass
//      before any merge / continuation.
//   4a. If verify green: git_commit { message } — squash commit on branch.
//   4b. If verify red:   git_rollback — hard-reset to the checkpoint sha.
//
// These are deliberately thin shells around `git` so the agent can still
// compose ad-hoc git operations via the Bash tool; the tools just give it
// a structured, named interface for the happy path.

// ─── git_checkpoint ──────────────────────────────────────────────────────

type GitCheckpointTool struct{ workspace string }

type gitCheckpointArgs struct {
	Message string `json:"message" validate:"required"`
	Branch  string `json:"branch,omitempty"` // override auto-generated name
}

func NewGitCheckpointTool(workspace string) *GitCheckpointTool {
	return &GitCheckpointTool{workspace: workspace}
}

func (t *GitCheckpointTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "git_checkpoint",
		Description: "Create a safety checkpoint before self-modifying code: stage all changes, commit with the given message on an autonomous/<ts> branch, and push to origin. Returns the commit SHA (use it with git_rollback on failure). Requires GITHUB_TOKEN in env.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "description": "Commit message summarising intended change."},
				"branch":  map[string]any{"type": "string", "description": "Optional branch name; auto-generated if empty."},
			},
			"required": []string{"message"},
		},
	}
}

func (t *GitCheckpointTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args gitCheckpointArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{ToolName: "git_checkpoint"}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Message) == "" {
		return failGit("git_checkpoint", "message is required"), nil
	}
	branch := args.Branch
	if branch == "" {
		branch = "autonomous/" + timestamp()
	}

	if strings.HasPrefix(branch, "-") {
		return failGit("git_checkpoint", "branch cannot start with '-' (possible argument injection)"), nil
	}

	// Create branch, stage, commit, push. Use `-A` so untracked files go
	// too — autonomous runs often generate new files we must preserve.
	steps := [][]string{
		{"git", "checkout", "-B", branch},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "autosnapshot: " + args.Message, "--allow-empty"},
	}
	for _, cmd := range steps {
		if out, err := runGit(ctx, t.workspace, cmd...); err != nil {
			return failGit("git_checkpoint", fmt.Sprintf("%s failed: %v\n%s", strings.Join(cmd, " "), err, out)), nil
		}
	}
	sha, _ := runGit(ctx, t.workspace, "git", "rev-parse", "HEAD")
	sha = strings.TrimSpace(sha)

	// Push best-effort — if no remote / no token, surface the error but
	// keep the local commit so rollback still works offline.
	if pushOut, err := runGit(ctx, t.workspace, "git", "push", "-u", "origin", branch); err != nil {
		return Result{
			ToolName: "git_checkpoint",
			OK:       true,
			Output:   fmt.Sprintf("checkpoint committed locally at %s on %s (push failed: %v)\n%s", sha, branch, err, pushOut),
			Metadata: map[string]any{"sha": sha, "branch": branch, "pushed": false},
		}, nil
	}
	return Result{
		ToolName: "git_checkpoint",
		OK:       true,
		Output:   fmt.Sprintf("checkpoint %s pushed to origin/%s", sha, branch),
		Metadata: map[string]any{"sha": sha, "branch": branch, "pushed": true},
	}, nil
}

// ─── git_verify ──────────────────────────────────────────────────────────

type GitVerifyTool struct{ workspace string }

func NewGitVerifyTool(workspace string) *GitVerifyTool {
	return &GitVerifyTool{workspace: workspace}
}

func (t *GitVerifyTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "git_verify",
		Description: "Verify current working tree builds and tests pass: runs `go build ./...` then `go test ./...`. Call after any code modification and before git_commit / merging. Returns OK=false if either fails; in that case call git_rollback.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (t *GitVerifyTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	// Fix bug-16: GoToolchainPolicy memasukkan PATH, GOPATH, GOROOT,
	// GOCACHE, TMP, LOCALAPPDATA dll ke env whitelist. Tanpa ini `go build`
	// gagal silent karena module cache tidak reachable.
	goPolicy := sandbox.GoToolchainPolicy(t.workspace, 5*time.Minute)
	buildRes, err := sandbox.RunArgv(ctx, goPolicy, "go", "build", "./...")
	if err != nil || buildRes.ExitCode != 0 {
		out := "go build failed:\n"
		if buildRes != nil {
			out += buildRes.Stdout + "\n" + buildRes.Stderr
		} else if err != nil {
			out += err.Error()
		}
		return Result{
			ToolName: "git_verify",
			OK:       false,
			Output:   out,
			Metadata: map[string]any{"stage": "build"},
		}, nil
	}
	testRes, err := sandbox.RunArgv(ctx, goPolicy, "go", "test", "./...")
	if err != nil || testRes.ExitCode != 0 {
		out := "go test failed:\n"
		if testRes != nil {
			out += testRes.Stdout + "\n" + testRes.Stderr
		} else if err != nil {
			out += err.Error()
		}
		return Result{
			ToolName: "git_verify",
			OK:       false,
			Output:   out,
			Metadata: map[string]any{"stage": "test"},
		}, nil
	}
	return Result{
		ToolName: "git_verify",
		OK:       true,
		Output:   "go build ./... OK; go test ./... OK",
		Metadata: map[string]any{"stage": "green"},
	}, nil
}

// ─── git_rollback ────────────────────────────────────────────────────────

type GitRollbackTool struct{ workspace string }

type gitRollbackArgs struct {
	Sha string `json:"sha" validate:"required"`
}

func NewGitRollbackTool(workspace string) *GitRollbackTool {
	return &GitRollbackTool{workspace: workspace}
}

func (t *GitRollbackTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name:        "git_rollback",
		Description: "Hard-reset the working tree to a checkpoint SHA. Use this when git_verify fails after self-modification. Accepts the SHA returned by git_checkpoint.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sha": map[string]any{"type": "string", "description": "Commit SHA to reset to."},
			},
			"required": []string{"sha"},
		},
	}
}

func (t *GitRollbackTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args gitRollbackArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{ToolName: "git_rollback"}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	if strings.TrimSpace(args.Sha) == "" {
		return failGit("git_rollback", "sha is required"), nil
	}
	// EXTBUG-004 fix: enforce strict SHA hex format. Previously any string
	// (including `--hard --`, `origin/main`, `HEAD~99`) went straight to
	// `git reset --hard`, giving an agent argument injection / unintended
	// rollback targets. `git_checkpoint` already validates branch names the
	// same way; rollback needs symmetric treatment.
	if !shaFormatRE.MatchString(args.Sha) {
		return failGit("git_rollback", "invalid SHA format (expect 7-40 hex chars)"), nil
	}
	if out, err := runGit(ctx, t.workspace, "git", "reset", "--hard", args.Sha); err != nil {
		return failGit("git_rollback", fmt.Sprintf("reset failed: %v\n%s", err, out)), nil
	}
	// Bug 1.3 fix (Gemini audit): `git clean -fd` was unconditionally removing
	// ALL untracked files — potentially wiping user work-in-progress that lived
	// next to agent edits. Rollback should restore tracked state only; stray
	// untracked files are the owner's problem. If needed, owner can manually
	// run `git clean` after confirming.
	return Result{
		ToolName: "git_rollback",
		OK:       true,
		Output:   "rolled back to " + args.Sha,
		Metadata: map[string]any{"sha": args.Sha},
	}, nil
}

// ─── helpers ─────────────────────────────────────────────────────────────

func runGit(ctx context.Context, workspace string, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command")
	}

	var safeArgs []string
	if args[0] == "git" {
		// Gemini audit fix: Disable git hooks to prevent privilege escalation
		// / sandbox escape via malicious .git/hooks/ scripts created by AI.
		safeArgs = append(safeArgs, "-c", "core.hooksPath=/dev/null")
	}
	safeArgs = append(safeArgs, args[1:]...)

	cmd := exec.CommandContext(ctx, args[0], safeArgs...)
	cmd.Dir = workspace
	// BUG-H19 fix (2026-04-19): filter os.Environ() ke allowlist supaya API keys
	// (ANTHROPIC, AWS, OPENROUTER, dsb) GAK bocor ke subprocess git. Hanya var
	// yang git butuhkan buat jalan (PATH, HOME, SSH, PROXY, locale) yang lolos.
	cmd.Env = append(safeGitEnv(), buildGitEnv()...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// safeGitEnv returns a filtered subset of os.Environ() containing only
// variables git legitimately needs. Keeps API keys + credentials OUT of the
// git subprocess to prevent leakage via a malicious git alias or hook.
func safeGitEnv() []string {
	allowed := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "USERNAME": true,
		"USERPROFILE": true, "HOMEPATH": true, "HOMEDRIVE": true,
		"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true,
		"TEMP": true, "TMP": true, "TMPDIR": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true, "LC_MESSAGES": true,
		"TZ": true, "TERM": true, "SHELL": true, "PWD": true,
		"SSH_AUTH_SOCK": true, "SSH_AGENT_PID": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
		"http_proxy": true, "https_proxy": true, "no_proxy": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "COMSPEC": true,
		"PATHEXT": true, "WINDIR": true, "PROCESSOR_ARCHITECTURE": true,
	}
	src := os.Environ()
	filtered := make([]string, 0, len(src)/4)
	for _, kv := range src {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		key := kv[:eq]
		if allowed[key] || strings.HasPrefix(key, "GIT_") {
			filtered = append(filtered, kv)
		}
	}
	return filtered
}

// buildGitEnv wires GITHUB_TOKEN (case-insensitive) from .env / process env
// into a GIT_ASKPASS helper so `git push` over HTTPS doesn't hang prompting
// for credentials inside an autonomous loop.
//
// Security (Gemini audit bugs 1.1 + 1.2):
//   - Script lives in the PROCESS-PRIVATE temp dir (os.TempDir), NEVER in
//     the workspace — prevents accidental git-commit of token via `git add .`.
//   - Cross-OS: on Windows writes a .bat, elsewhere a .sh (executable).
//   - Caller should invoke CleanupAskpass() at process exit.
var (
	askpassOnce sync.Once
	askpassPath string
)

func buildGitEnv() []string {
	token := firstEnv("GITHUB_TOKEN", "github_token")
	if token == "" {
		return nil
	}
	askpassOnce.Do(func() {
		askpassPath = writeAskpass()
	})
	if askpassPath == "" {
		return nil
	}
	return []string{
		"GIT_ASKPASS=" + askpassPath,
		"GIT_TERMINAL_PROMPT=0",
		"FLOWORK_GIT_TOKEN=" + token,
	}
}

// writeAskpass creates a process-private askpass helper outside the
// workspace. It NO LONGER embeds the token directly (Bug 37 fix). Instead,
// it simply echoes the FLOWORK_GIT_TOKEN environment variable. This prevents
// CMD/Shell injection from a malicious or malformed token string.
func writeAskpass() string {
	dir, err := os.MkdirTemp("", "flowork-askpass-")
	if err != nil {
		return ""
	}
	var path, script string
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "askpass.bat")
		script = "@echo off\r\necho %FLOWORK_GIT_TOKEN%\r\n"
	} else {
		path = filepath.Join(dir, "askpass.sh")
		script = "#!/bin/sh\nprintf '%s\\n' \"$FLOWORK_GIT_TOKEN\"\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return ""
	}
	return path
}

// CleanupAskpass removes the temp askpass script. Call at shutdown.
func CleanupAskpass() {
	if askpassPath != "" {
		_ = os.Remove(askpassPath)
		_ = os.Remove(filepath.Dir(askpassPath))
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func timestamp() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

func failGit(name, msg string) Result {
	return Result{ToolName: name, OK: false, Output: msg}
}
