// === LOCKED FILE ===
// Status: STABLE — computer-operator feature, tested end-to-end 2026-06-08. Allowlist desktop-app launcher (exec:app).
// Do not edit without owner approval.

// app_open.go — launch a whitelisted desktop app on the host (Chrome, VS Code, …).
//
// Companion to system_power for the operator agent: lets the owner say "buka
// chrome" / "open vscode" from Telegram and have it open on the PC. SAFE BY
// ALLOWLIST: the user only ever picks an allowlist KEY ("chrome"); the actual
// command is fixed by the owner, never built from chat text — so even a prompt-
// injected request can't launch an arbitrary binary (no injection surface, argv
// exec, no shell on Linux/macOS). Requires the exec:app capability (privileged
// operator agent only). Owner-extensible: drop ~/.flowork/operator-apps.json to
// add or override apps.
//
// Multi-OS: Linux (binary on PATH), macOS (open -a), Windows (start). Linux is
// the tested path; Windows/macOS are written but unverified on a real machine.
package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"flowork-gui/internal/tools"
)

// appDefaults — built-in allowlist. key → GOOS → ordered candidate commands.
// First candidate found on PATH wins (macOS uses the app name with `open -a`).
var appDefaults = map[string]map[string][]string{
	"chrome": {
		"linux":   {"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"},
		"windows": {"chrome"},
		"darwin":  {"Google Chrome"},
	},
	"code": {
		"linux":   {"code", "codium"},
		"windows": {"code.cmd", "code"},
		"darwin":  {"Visual Studio Code"},
	},
}

// appAliases — friendly names → allowlist key.
var appAliases = map[string]string{
	"chrome": "chrome", "google chrome": "chrome", "google-chrome": "chrome", "browser": "chrome",
	"code": "code", "vscode": "code", "vs code": "code", "visual studio code": "code", "vsc": "code",
}

// loadAppAllow merges the built-in allowlist with an owner-editable
// ~/.flowork/operator-apps.json (same shape as appDefaults). Owner entries win.
func loadAppAllow() map[string]map[string][]string {
	out := map[string]map[string][]string{}
	for k, v := range appDefaults {
		out[k] = v
	}
	if home, err := os.UserHomeDir(); err == nil {
		if raw, err := os.ReadFile(filepath.Join(home, ".flowork", "operator-apps.json")); err == nil {
			var extra map[string]map[string][]string
			if json.Unmarshal(raw, &extra) == nil {
				for k, v := range extra {
					out[strings.ToLower(strings.TrimSpace(k))] = v
				}
			}
		}
	}
	return out
}

type appOpenTool struct{}

func (appOpenTool) Name() string       { return "app_open" }
func (appOpenTool) Capability() string { return "exec:app" }
func (appOpenTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Launch a whitelisted desktop app on the host computer (e.g. chrome, code). Safe: only apps in the owner's allowlist can be opened — the request can never run an arbitrary command. Requires exec:app (operator agent). Use for 'open/buka <app>' requests.",
		Params: []tools.Param{
			{Name: "app", Type: tools.ParamString, Description: "app to open: chrome | code (aliases: vscode, browser, …)", Required: true},
		},
		Returns: "{opened, app, command} — or {opened:false, error, allowed:[…]} if not in the allowlist / not installed",
	}
}

func (appOpenTool) Run(_ context.Context, args map[string]any) (tools.Result, error) {
	raw, _ := args["app"].(string)
	want := strings.ToLower(strings.TrimSpace(raw))
	if want == "" {
		return tools.Result{}, fmt.Errorf("app required")
	}
	allow := loadAppAllow()
	key, ok := appAliases[want]
	if !ok {
		if _, direct := allow[want]; direct {
			key = want // a key added via operator-apps.json without an alias
		}
	}
	allowedKeys := make([]string, 0, len(allow))
	for k := range allow {
		allowedKeys = append(allowedKeys, k)
	}
	perOS, known := allow[key]
	if key == "" || !known {
		return tools.Result{Output: map[string]any{"opened": false, "error": "app not in allowlist", "requested": raw, "allowed": allowedKeys}}, nil
	}
	cands := perOS[runtime.GOOS]
	if len(cands) == 0 {
		return tools.Result{Output: map[string]any{"opened": false, "error": "app not configured for this OS (" + runtime.GOOS + ")", "app": key}}, nil
	}

	var cmd *exec.Cmd
	var resolved string
	switch runtime.GOOS {
	case "darwin":
		resolved = cands[0]
		cmd = exec.Command("open", "-a", resolved)
	case "windows":
		for _, c := range cands {
			if p, err := exec.LookPath(c); err == nil {
				resolved = p
				break
			}
		}
		if resolved == "" {
			resolved = cands[0] // let `start` resolve via App Paths (e.g. chrome)
		}
		// argv fixed from the allowlist (not chat) → no injection.
		cmd = exec.Command("cmd", "/c", "start", "", resolved)
	default: // linux & others
		for _, c := range cands {
			if p, err := exec.LookPath(c); err == nil {
				resolved = p
				break
			}
		}
		if resolved == "" {
			return tools.Result{Output: map[string]any{"opened": false, "error": "not installed (none of the candidates found on PATH)", "app": key, "candidates": cands}}, nil
		}
		cmd = exec.Command(resolved)
	}
	// Detach: don't wait, release the process so it outlives this call.
	if err := cmd.Start(); err != nil {
		return tools.Result{Output: map[string]any{"opened": false, "error": err.Error(), "app": key, "command": resolved}}, nil
	}
	_ = cmd.Process.Release()
	return tools.Result{
		Output: map[string]any{"opened": true, "app": key, "command": resolved},
		Note:   "launched " + key,
	}, nil
}
