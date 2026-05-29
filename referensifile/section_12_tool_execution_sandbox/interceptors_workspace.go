package tools

// interceptors_workspace.go — WorkspaceInterceptor.
// Mencegah path file dan command lolos keluar workspace via SafeJoin.
//
// Gemini audit fix (Bug 2.2): all file-mutation/read tools validated
// (read/write/edit/multiedit/notebookedit/glob/list).
//
// Gemini audit #5 (workspace-escape via unchecked keys): iterate broad
// pathKeys list AND scan every string-valued arg that looks path-shaped.

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
)

// WorkspaceInterceptor mencegah path file dan command lolos keluar workspace.
//
// Per Ayah arahan 2026-05-08 SAKRAL: warga umum (non-coder) HANYA boleh
// write/edit/bash di kamar mereka sendiri `workspaces/<warga>/`. Coder
// (aksara/wiraga/kembar) special — bisa akses LUAR workspace (full project
// access untuk perbaiki kode).
//
// Goal: AI mandiri tanpa Ayah, tapi tetap sandboxed per role. Warga yang
// bukan coder ngga bisa rusakin file luar kamar mereka.
type WorkspaceInterceptor struct {
	Root string
}

// coderWhitelist — warga yang punya hak write/edit/bash full project access.
// Sinkron sama wargaregistry.ActiveCoders() (FIX #7), tapi di-duplicate di sini
// supaya tools package ngga butuh extra import + ngga circular.
var coderWhitelist = map[string]bool{
	"aksara": true,
	"wiraga": true,
	"kembar": true,
}

// IsCoder reports whether agent has full project access (bypass workspace
// kamar restriction). Used by Before() saat warga umum coba write/edit/bash
// luar `workspaces/<self>/`.
func IsCoder(agent string) bool {
	return coderWhitelist[strings.ToLower(strings.TrimSpace(agent))]
}

// validateWargaKamar — kalau agent BUKAN coder + tool mutation (write/edit/
// bash), path WAJIB di-resolve dalam `<root>/workspaces/<agent>/`. Coder
// bypass (bisa luar). Kalau path ada di luar workspaces/<agent>/ untuk warga
// umum, return error edukasi.
//
// Special-case: agent "default" / "" dianggap warga umum (most restrictive).
func (i WorkspaceInterceptor) validateWargaKamar(agent, path string) error {
	if IsCoder(agent) {
		return nil // coder full access, skip kamar boundary
	}
	if strings.TrimSpace(agent) == "" {
		agent = "default"
	}
	kamarRoot := filepath.Join(i.Root, "workspaces", agent)
	resolved, err := SafeJoin(i.Root, path)
	if err != nil {
		return err // already escaped workspace root, error edukasi already wrapped
	}
	kamarAbs, _ := filepath.Abs(kamarRoot)
	rel, err := filepath.Rel(kamarAbs, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		edu := braindb.GetEducationalError(i.Root, "ERR_WORKSPACE_NOT_FOUND",
			fmt.Sprintf("warga %q hanya boleh write/edit/bash di kamar `workspaces/%s/`. Path %q ada di luar kamar lo. Kalau lo butuh akses luar kamar, escalate ke coder (aksara/wiraga/kembar) via tool_propose atau bug_report.", agent, agent, path))
		return fmt.Errorf("%s\n\n[teknis: warga=%q kamar=%q escape ke %q]", edu, agent, kamarRoot, resolved)
	}
	return nil
}

// Before memeriksa sebelum eksekusi apakah argumen path masih berada di dalam workspace.
//
// Gemini audit fix (Bug 2.2): previously only "filesystem" and "bash" were
// checked. Agent could pass absolute paths like "C:\Windows\System32\..."
// or "/etc/passwd" via read/write/edit/multiedit/notebookedit and escape
// workspace boundary. All file-mutation/read tools now validated.
//
// Gemini audit #5 (workspace-escape via unchecked keys): earlier we only
// iterated [path, file_path]. Tools that also accept "destination",
// "target", "dst", "src", "to", "from" (filesystem move/copy, potential
// future tools) escaped validation. Now we iterate a broad key list AND
// additionally scan every string-valued arg that looks path-shaped
// (contains separator or filepath.IsAbs).
func (i WorkspaceInterceptor) Before(_ context.Context, invocation *Invocation) error {
	pathKeys := []string{
		"path", "file_path",
		"destination", "target", "dst", "src", "to", "from",
		"output_path", "input_path", "source", "source_path",
		"working_dir",
	}
	// Per Ayah arahan SAKRAL 2026-05-08: warga umum (non-coder) hanya boleh
	// MUTATION (write/edit/multiedit/notebookedit) di kamar mereka. Read-only
	// tools (read/list/glob) tetap full project access. Coder bypass.
	mutationTools := map[string]bool{
		"write": true, "edit": true, "multiedit": true, "notebookedit": true,
	}
	// Per Ayah multi-tenant fix 2026-05-09: kernel inject `args["agent"] =
	// persona.ID` di process_message.go:178-179 + 595-596 sebelum dispatch
	// ke worker. ParsedArgs["agent"] = sumber identitas per-request yang
	// otoritatif. Fallback ke env-based currentAgent() untuk single-binary
	// legacy path (CLI tool, non-kernel-routed). Sebelumnya hanya env →
	// aksara coder ditolak sebagai "default" walau dispatch dari kernel
	// dengan persona.ID="aksara" benar (BUG #580/#575 root cause).
	agent := currentAgent()
	if argAgent, ok := stringArg(invocation.ParsedArgs, "agent"); ok && strings.TrimSpace(argAgent) != "" {
		agent = strings.TrimSpace(argAgent)
	}

	switch invocation.ToolName {
	case "filesystem", "read", "write", "edit", "multiedit", "notebookedit", "list", "glob":
		for _, key := range pathKeys {
			if path, ok := stringArg(invocation.ParsedArgs, key); ok && strings.TrimSpace(path) != "" {
				if _, err := SafeJoin(i.Root, path); err != nil {
					edu := braindb.GetEducationalError(i.Root, "ERR_WORKSPACE_NOT_FOUND", path)
					return fmt.Errorf("%s\n\n[teknis: tool=%q key=%s: %w]", edu, invocation.ToolName, key, err)
				}
				// Mutation tool + non-coder: must be in workspaces/<self>/.
				if mutationTools[invocation.ToolName] {
					if err := i.validateWargaKamar(agent, path); err != nil {
						return err
					}
				}
			}
		}
		// Belt & suspenders: scan any OTHER string arg that looks like a path.
		for k, v := range invocation.ParsedArgs {
			s, ok := v.(string)
			if !ok || strings.TrimSpace(s) == "" {
				continue
			}
			if containsKey(pathKeys, k) {
				continue // already handled above
			}
			if looksLikePath(s) {
				if _, err := SafeJoin(i.Root, s); err != nil {
					edu := braindb.GetEducationalError(i.Root, "ERR_WORKSPACE_NOT_FOUND", s)
					return fmt.Errorf("%s\n\n[teknis: tool=%q key=%s: %w]", edu, invocation.ToolName, k, err)
				}
			}
		}
	case "grep":
		if path, ok := stringArg(invocation.ParsedArgs, "path"); ok && strings.TrimSpace(path) != "" {
			if _, err := SafeJoin(i.Root, path); err != nil {
				edu := braindb.GetEducationalError(i.Root, "ERR_WORKSPACE_NOT_FOUND", path)
				return fmt.Errorf("%s\n\n[teknis: tool=grep: %w]", edu, err)
			}
		}
	case "bash":
		if workingDir, ok := stringArg(invocation.ParsedArgs, "working_dir"); ok && strings.TrimSpace(workingDir) != "" {
			// BUG-036 fix (2026-04-25): only return error when SafeJoin fails.
			if _, err := SafeJoin(i.Root, workingDir); err != nil {
				edu := braindb.GetEducationalError(i.Root, "ERR_WORKSPACE_NOT_FOUND", workingDir)
				return fmt.Errorf("%s\n\n[teknis: tool=bash working_dir: %w]", edu, err)
			}
			// Per Ayah SAKRAL 2026-05-08: warga non-coder bash HANYA dalam
			// kamar `workspaces/<self>/`. Coder bypass full project access.
			if err := i.validateWargaKamar(agent, workingDir); err != nil {
				return err
			}
		}
	default:
		// no-op — exhaustive switch guard
	}
	return nil
}

// containsKey reports whether k is in the keys slice. Small helper used
// by the workspace interceptor to avoid double-validation.
func containsKey(keys []string, k string) bool {
	for _, kk := range keys {
		if kk == k {
			return true
		}
	}
	return false
}

// looksLikePath is a conservative heuristic: a string is treated as a path
// worth validating if it contains a separator, is absolute, or starts with
// common path-escape patterns. Aggressive false-positive is acceptable
// because workspace validation is non-destructive — it only rejects paths
// that SafeJoin refuses.
func looksLikePath(s string) bool {
	if strings.ContainsAny(s, "/\\") {
		return true
	}
	if filepath.IsAbs(s) {
		return true
	}
	return false
}

// After adalah post-hook workspace interceptor; saat ini tidak melakukan apa pun.
func (WorkspaceInterceptor) After(_ context.Context, _ Invocation, _ *Result, _ error) {}

// SafeJoin membatasi candidate path agar tetap berada dalam workspace dan mengembalikan path absolut yang aman.
func SafeJoin(root string, candidate string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", root, err)
	}

	if strings.TrimSpace(candidate) == "" {
		candidate = "."
	}

	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}

	target := filepath.Clean(candidate)
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	} else if resolvedParent := walkUpAndResolve(target); len(resolvedParent) > 0 {
		target = resolvedParent
	}

	rel, err := filepath.Rel(rootAbs, target)
	if err != nil {
		return "", fmt.Errorf("resolve relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace %q", target, rootAbs)
	}
	return target, nil
}

// stringArg membaca string value dari parsed arguments secara aman.
func stringArg(arguments map[string]any, key string) (string, bool) {
	if arguments == nil {
		return "", false
	}
	value, ok := arguments[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}
