// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

const MaxBodyBytes = 64 * 1024

func LoadFromDir(dir string) (loaded int, skipped int, err error) {
	if dir == "" {
		return 0, 0, fmt.Errorf("dir required")
	}
	info, serr := os.Stat(dir)
	if serr != nil {
		if os.IsNotExist(serr) {
			return 0, 0, nil
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

	body := strings.ReplaceAll(c.BodyField, "{args}", argsRaw)
	return slashcmd.Result{Text: body, Format: "markdown"}, nil
}

func ParseMarkdown(raw, fallbackName string) (*CustomCommand, error) {
	cmd := &CustomCommand{NameField: fallbackName, BodyField: raw}
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---") {

		cmd.BodyField = raw
		cmd.NameField = fallbackName
		return cmd, validateName(cmd.NameField)
	}

	rest := raw[3:]
	rest = strings.TrimLeft(rest, "\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {

		cmd.BodyField = raw
		cmd.NameField = fallbackName
		return cmd, validateName(cmd.NameField)
	}
	front := rest[:end]
	body := rest[end+4:]
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
