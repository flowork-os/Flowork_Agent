package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/teetah2402/flowork/internal/mdloader"
)

// skillMeta mirrors the YAML frontmatter in SKILL.md files.
type skillMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Trigger (optional) — keywords/pattern yang menandai skill cocok.
	// Saat ini hanya informational; picker logic akan diperkaya nanti.
	Trigger string `yaml:"trigger"`
}

// Scope labels untuk skill markdown sources — single source of truth dipakai
// loadSkillRoot + RegisterUserSkill conflict warning.
const (
	skillScopeGlobal           = "global"            // ~/.flowork/skills/
	skillScopeCommitted        = "committed"         // <workspace>/skills/ (ADR-013)
	skillScopeWorkspacePrivate = "workspace-private" // <workspace>/.flowork/skills/
)

// LoadUserSkills scans 3 lokasi untuk SKILL.md dengan precedence eksplisit:
//
//  1. global ~/.flowork/skills/<n>/SKILL.md             — lowest precedence
//  2. committed <workspace>/skills/<n>/SKILL.md          — middle (ADR-013)
//  3. workspace-private <workspace>/.flowork/skills/<n>  — highest precedence
//
// Load order = lowest → highest, jadi last-write-wins matching effekdomino
// #56 solusi. Conflict (nama sama di 2+ scope) → log warning via
// RegisterUserSkill — operator bisa spot silent override pasca-restart.
//
// Returns jumlah skill user-defined yang berhasil dimuat.
func (r *SkillRegistry) LoadUserSkills(workspace string) int {
	loaded := 0
	if home, err := os.UserHomeDir(); err == nil {
		loaded += r.loadSkillRoot(filepath.Join(home, ".flowork", "skills"), skillScopeGlobal)
	}
	if workspace != "" {
		// ADR-013: committed workspace skills (skills/ di repo root).
		loaded += r.loadSkillRoot(filepath.Join(workspace, "skills"), skillScopeCommitted)
		// Workspace-private = highest precedence (load last → win).
		loaded += r.loadSkillRoot(filepath.Join(workspace, ".flowork", "skills"), skillScopeWorkspacePrivate)
	}
	return loaded
}

// loadSkillRoot membaca struktur <root>/<n>/SKILL.md — pola persis
// seperti Claude Code skills marketplace. Folder tanpa SKILL.md di-skip.
// scope label dipakai untuk conflict warning saat nama collision (#56).
func (r *SkillRegistry) loadSkillRoot(root, scope string) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(root, e.Name(), "SKILL.md")
		doc, err := mdloader.LoadFile[skillMeta](path)
		if err != nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(doc.Meta.Name))
		if name == "" {
			name = strings.ToLower(e.Name())
		}
		r.RegisterUserSkill(Skill{
			Name:        name,
			Description: doc.Meta.Description,
			Prompt:      strings.TrimSpace(doc.Body),
		}, scope)
		n++
	}
	return n
}

// DescribeUserSkills — untuk /skills output membedakan built-in vs user.
// Dikembalikan sebagai blok teks siap append.
func DescribeUserSkillsHint(workspace string) string {
	var hints []string
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".flowork", "skills")); err == nil {
			hints = append(hints, "~/.flowork/skills/")
		}
	}
	if workspace != "" {
		if _, err := os.Stat(filepath.Join(workspace, ".flowork", "skills")); err == nil {
			hints = append(hints, filepath.Join(workspace, ".flowork", "skills"))
		}
	}
	if len(hints) == 0 {
		return "(untuk tambah skill custom, bikin folder di ~/.flowork/skills/<nama>/SKILL.md)"
	}
	return fmt.Sprintf("skill dirs: %s", strings.Join(hints, ", "))
}
