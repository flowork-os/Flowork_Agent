// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Brain drawer/embeddings/skills.
// 2026-06-15 (owner-approved, skill_author): SelectSkills now ALSO merges DYNAMIC
//   skills authored at runtime (SKILL.md files under DynamicSkillsDir()) on top of the
//   embedded library — so agents the Architect builds can ship reusable skills that get
//   injected. FAIL-OPEN: any read/parse error just skips dynamic skills (embedded still
//   works). Read fresh per call (skills are few + small) so new ones appear w/o restart.
// 2026-06-15 (owner-approved, R4 extension points): allSkills() kini iterasi REGISTRY
//   SkillProvider (lihat skill_provider.go) — embedded + dynamic jadi provider builtin;
//   nambah sumber skill = RegisterSkillProvider, bukan edit core. Behavior identik.

// Embedded skill library + selector (+ dynamic on-disk skills, + provider registry R4).

package brain

import (
	"embed"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// DynamicSkillsDir resolves the writable dir for runtime-authored skills (SKILL.md).
// Precedence mirrors the brain DB: $FLOW_ROUTER_DATA/skills → ~/.flow_router/skills.
// The skill_author endpoint writes here; SelectSkills reads here. "" if unresolvable.
func DynamicSkillsDir() string {
	if d := os.Getenv("FLOW_ROUTER_DATA"); d != "" {
		return filepath.Join(d, "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flow_router", "skills")
}

// loadDynamicSkills reads runtime skills from DynamicSkillsDir (each *.md or
// <name>/SKILL.md). FAIL-OPEN: returns nil on any error so embedded skills still load.
func loadDynamicSkills() []SkillDoc {
	dir := DynamicSkillsDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // dir missing / unreadable → no dynamic skills (fine)
	}
	var out []SkillDoc
	for _, e := range entries {
		var path, fname string
		if e.IsDir() {
			path = filepath.Join(dir, e.Name(), "SKILL.md")
			fname = e.Name() + ".md"
		} else if strings.HasSuffix(e.Name(), ".md") {
			path = filepath.Join(dir, e.Name())
			fname = e.Name()
		} else {
			continue
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil || len(raw) == 0 {
			continue
		}
		out = append(out, parseSkill(string(raw), fname))
	}
	return out
}

// allSkills = gabungan SEMUA SkillProvider terdaftar (embedded + dynamic on-disk +
// eksternal masa depan), deduped by name (provider lebih awal menang). Lihat
// skill_provider.go (R4 registry). Nambah sumber = RegisterSkillProvider, bukan edit sini.
func allSkills() []SkillDoc {
	seen := map[string]bool{}
	var out []SkillDoc
	for _, p := range skillProviders {
		for _, s := range p() {
			if s.Name == "" {
				continue
			}
			k := strings.ToLower(s.Name)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, s)
		}
	}
	return out
}

// skilldata holds the behavioral skill library, embedded into the binary so the
// brain's "skills" travel with Flow Router — any agent that hits the endpoint
// gets the same skill set, no external files required (plug-and-play).
//
//go:embed skilldata/*.md
var skilldata embed.FS

// SkillDoc — one behavioral skill: frontmatter name/description + markdown body.
type SkillDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"` // full markdown (frontmatter stripped)
}

var (
	skillsOnce   sync.Once
	skillsCached []SkillDoc
)

// Skills returns the parsed, embedded skill library (loaded once).
func Skills() []SkillDoc {
	skillsOnce.Do(func() {
		entries, _ := skilldata.ReadDir("skilldata")
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			raw, err := skilldata.ReadFile("skilldata/" + e.Name())
			if err != nil {
				continue
			}
			skillsCached = append(skillsCached, parseSkill(string(raw), e.Name()))
		}
		sort.Slice(skillsCached, func(i, j int) bool { return skillsCached[i].Name < skillsCached[j].Name })
	})
	return skillsCached
}

// parseSkill extracts the YAML-ish frontmatter (name, description) and body.
// Falls back to the filename for name if no frontmatter is present.
func parseSkill(raw, filename string) SkillDoc {
	doc := SkillDoc{Name: strings.TrimSuffix(filename, ".md"), Body: raw}
	if !strings.HasPrefix(raw, "---") {
		return doc
	}
	end := strings.Index(raw[3:], "\n---")
	if end < 0 {
		return doc
	}
	front := raw[3 : end+3]
	doc.Body = strings.TrimLeft(raw[end+3+4:], "\n")
	for _, line := range strings.Split(front, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			doc.Name = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(line, "description:"); ok {
			doc.Description = strings.TrimSpace(v)
		}
	}
	return doc
}

// SelectSkills ranks the skill library against a query by keyword overlap and
// returns the top-N most relevant skills. Cheap, deterministic, no embeddings:
// score = (matches in name ×3) + (matches in description ×2) + (matches in body).
// Returns nil when nothing meaningfully overlaps, so irrelevant skills are not
// injected.
func SelectSkills(query string, limit int) []SkillDoc {
	if limit <= 0 {
		limit = 3
	}
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		doc   SkillDoc
		score int
	}
	var ranked []scored
	for _, s := range allSkills() {
		name := strings.ToLower(s.Name)
		desc := strings.ToLower(s.Description)
		body := strings.ToLower(s.Body)
		score := 0
		for _, t := range terms {
			score += 3 * strings.Count(name, t)
			score += 2 * strings.Count(desc, t)
			if strings.Contains(body, t) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{s, score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	var out []SkillDoc
	for i := 0; i < len(ranked) && i < limit; i++ {
		out = append(out, ranked[i].doc)
	}
	return out
}

// queryTerms lowercases the query and returns distinct tokens >=3 chars,
// skipping a few common stopwords so they don't dominate scoring.
func queryTerms(query string) []string {
	stop := map[string]bool{"the": true, "and": true, "for": true, "yang": true, "dan": true, "untuk": true, "dengan": true}
	seen := map[string]bool{}
	var out []string
	for _, f := range strings.Fields(strings.ToLower(query)) {
		t := strings.Trim(f, ".,:;!?\"'()[]{}")
		if len(t) < 3 || stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
