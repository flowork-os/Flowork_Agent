// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 16 phase 1 (Custom slash command loader). API stable:
//   LoadFromDir scans .md files, parses YAML-ish frontmatter (name,
//   aliases, description), registers via slashcmd.Register. Body served
//   as template — {args} placeholder replaced dengan caller's argsRaw.
//   Phase 2+ (hot-reload fsnotify, multi-file include, body run via
//   LLM) → tambah file baru, JANGAN modify ini.
//
// Package custom — Section 16 phase 1: file-backed slash commands.
//
// FORMAT .md:
//
//   ---
//   name: rules
//   aliases: [r, rule]
//   description: Show project rules
//   ---
//
//   Project rules:
//   1. Lock files when stable
//   2. Test before claim done
//   3. {args}    ← replaced dengan caller's argsRaw
//
// Filename (.md stripped, lowercase) di-pakai sebagai default name
// kalau frontmatter `name:` ngga ada.

package custom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowork-gui/internal/slashcmd"
)

// MaxBodyBytes — hard cap body file supaya ngga blow up memory.
const MaxBodyBytes = 64 * 1024

// LoadFromDir — scan directory rekursif (level 1 only), parse setiap .md
// file, register slash command. Return (loaded count, skipped count, error).
//
// Skip kalau filename bukan .md, body > MaxBodyBytes, atau frontmatter
// invalid. Panic kalau name collision (consistent dengan slashcmd.Register).
func LoadFromDir(dir string) (loaded int, skipped int, err error) {
	if dir == "" {
		return 0, 0, fmt.Errorf("dir required")
	}
	info, serr := os.Stat(dir)
	if serr != nil {
		if os.IsNotExist(serr) {
			return 0, 0, nil // empty / not yet created
		}
		return 0, 0, fmt.Errorf("stat dir: %w", serr)
	}
	if !info.IsDir() {
		return 0, 0, fmt.Errorf("not a directory: %s", dir)
	}

	entries, rerr := os.ReadDir(dir)
	if rerr != nil {
		return 0, 0, fmt.Errorf("read dir: %w", rerr)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			skipped++
			continue
		}
		full := filepath.Join(dir, e.Name())
		// Skip symlink (anti symlink follow leak).
		if fi, lerr := os.Lstat(full); lerr == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				skipped++
				continue
			}
		}

		raw, rerr := os.ReadFile(full)
		if rerr != nil {
			skipped++
			continue
		}
		if len(raw) > MaxBodyBytes {
			skipped++
			continue
		}

		filenameBase := strings.TrimSuffix(strings.ToLower(e.Name()), ".md")
		cmd, perr := ParseMarkdown(string(raw), filenameBase)
		if perr != nil {
			skipped++
			continue
		}
		slashcmd.Register(cmd)
		loaded++
	}
	return loaded, skipped, nil
}

// CustomCommand — file-backed SlashCommand. Body served sebagai template.
type CustomCommand struct {
	NameField    string
	AliasesField []string
	DescField    string
	BodyField    string
}

func (c *CustomCommand) Name() string        { return c.NameField }
func (c *CustomCommand) Aliases() []string   { return c.AliasesField }
func (c *CustomCommand) Description() string { return c.DescField }
func (c *CustomCommand) Run(_ context.Context, argsRaw string) (slashcmd.Result, error) {
	// Replace {args} placeholder dengan argsRaw. Single-pass — caller
	// bisa template "{args}" multiple kali, semua ke-replace.
	body := strings.ReplaceAll(c.BodyField, "{args}", argsRaw)
	return slashcmd.Result{Text: body, Format: "markdown"}, nil
}

// ParseMarkdown — parse YAML-ish frontmatter + body. Format:
//   ---
//   name: rules
//   aliases: [r, rule]    (atau "aliases: r, rule")
//   description: Show rules
//   ---
//   <body markdown>
//
// fallbackName dipakai kalau frontmatter `name:` ngga ada.
func ParseMarkdown(raw, fallbackName string) (*CustomCommand, error) {
	cmd := &CustomCommand{NameField: fallbackName, BodyField: raw}
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---") {
		// No frontmatter — pakai fallback name, body raw.
		cmd.BodyField = raw
		cmd.NameField = fallbackName
		return cmd, validateName(cmd.NameField)
	}
	// Skip leading "---" + newline.
	rest := raw[3:]
	rest = strings.TrimLeft(rest, "\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		// Malformed — treat raw as body, fallback name.
		cmd.BodyField = raw
		cmd.NameField = fallbackName
		return cmd, validateName(cmd.NameField)
	}
	front := rest[:end]
	body := rest[end+4:] // skip "\n---"
	body = strings.TrimLeft(body, "\r\n")

	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "name":
			cmd.NameField = strings.ToLower(strings.TrimSpace(val))
		case "aliases":
			cmd.AliasesField = parseAliases(val)
		case "description":
			cmd.DescField = val
		}
	}
	cmd.BodyField = strings.TrimSpace(body)
	if cmd.NameField == "" {
		cmd.NameField = fallbackName
	}
	return cmd, validateName(cmd.NameField)
}

// parseAliases — "[a, b]" atau "a, b" → ["a", "b"].
func parseAliases(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		p = strings.ToLower(p)
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// validateName — alphanumeric + dash + underscore. Anti-conflict dengan
// dispatcher parse (split di space/tab).
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name empty")
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return fmt.Errorf("invalid char in name %q (alphanumeric + dash + underscore only)", name)
		}
	}
	return nil
}
