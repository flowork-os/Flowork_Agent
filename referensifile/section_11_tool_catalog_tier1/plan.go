package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/fsutil"
	"github.com/teetah2402/flowork/internal/provider"
)

// ADR-016 Perencanaan Nusantara — tool untuk warga tulis/baca roadmap
// di docs/plan/. Wangsit nulis bersama/*, 17 warga nulis warga/<persona>/*.

// validPersonas — 17 persona per ADR-013 yang boleh punya folder roadmap.
// Dipakai plan_write untuk validate input.
var validPlanPersonas = map[string]bool{
	// AI agency (12)
	"aksara": true, "wiraga": true, "wira-eka": true, "wira-dwi": true, "wira-tri": true,
	"wangsit": true, "kembar": true, "pramudita": true, "selam": true, "nyawang": true,
	"ombak": true, "flowork-bot": true,
	// Infrastructure (5)
	"jejak": true, "merpati": true, "balai": true, "gerbang": true, "flowork": true,
}

// PlanWriteTool — tulis/update roadmap ke docs/plan/.
type PlanWriteTool struct {
	workspace string
}

type planWriteArgs struct {
	Scope      string   `json:"scope" validate:"required"`                 // "bersama" | "warga"
	Persona    string   `json:"persona,omitempty"`     // required if scope=warga; required="wangsit" if scope=bersama
	Period     string   `json:"period"`                // "yearly" | "monthly" | "daily"
	Body       string   `json:"body" validate:"required"`                  // markdown body (tanpa frontmatter)
	Status     string   `json:"status,omitempty"`      // daily only: "DONE"|"WIP"|"BLOCKED"
	AlignedTo  []string `json:"aligned_to,omitempty"`  // reference parent roadmap
	DeriveFrom string   `json:"derive_from,omitempty"` // daily only: "monthly" — auto-derive dari monthly
	Date       string   `json:"date,omitempty"`        // override tanggal (default: today sesuai period)
}

func NewPlanWriteTool(workspace string) *PlanWriteTool {
	return &PlanWriteTool{workspace: workspace}
}

func (t *PlanWriteTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "plan_write",
		Description: `Tulis/update roadmap ke docs/plan/ (ADR-016 Perencanaan Nusantara).

Dua scope:
  - "bersama": roadmap kolektif. HANYA Wangsit yang boleh tulis (persona="wangsit").
  - "warga":   roadmap personal per-warga. Tulis ke folder lo sendiri.

Tiga period:
  - "yearly":  visi tahun ini. date default "YYYY".
  - "monthly": milestone bulan ini. date default "YYYY-MM".
  - "daily":   fokus hari ini. date default "YYYY-MM-DD". Wajib status=DONE|WIP|BLOCKED.

Required:
  scope, period, body
  + persona (untuk scope=warga, atau =wangsit untuk scope=bersama)

Optional:
  status (daily only), aligned_to (array ref roadmap parent), derive_from="monthly" (auto-scaffold daily dari monthly), date (override).

File ditulis ke:
  bersama/<period>.md             (scope=bersama)
  warga/<persona>/<period>.md     (scope=warga)

Frontmatter YAML di-generate otomatis, lo cukup tulis body markdown.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope":       map[string]any{"type": "string", "enum": []string{"bersama", "warga"}},
				"persona":     map[string]any{"type": "string", "description": "Persona identifier. Required kalau scope=warga. Harus 'wangsit' kalau scope=bersama."},
				"period":      map[string]any{"type": "string", "enum": []string{"yearly", "monthly", "daily"}},
				"body":        map[string]any{"type": "string", "description": "Markdown body. Frontmatter auto-generate."},
				"status":      map[string]any{"type": "string", "enum": []string{"DONE", "WIP", "BLOCKED"}, "description": "Daily only."},
				"aligned_to":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Reference parent roadmap, e.g. 'warga/aksara/monthly.md'."},
				"derive_from": map[string]any{"type": "string", "enum": []string{"monthly"}, "description": "Daily only — auto-scaffold dari monthly."},
				"date":        map[string]any{"type": "string", "description": "Override tanggal. Default: today sesuai period."},
			},
			"required": []string{"scope", "period", "body"},
		},
	}
}

func (t *PlanWriteTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args planWriteArgs
	if err := json.Unmarshal(inv.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("plan_write: invalid args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }


	scope := strings.ToLower(strings.TrimSpace(args.Scope))
	period := strings.ToLower(strings.TrimSpace(args.Period))
	persona := sanitizePersonaName(strings.TrimSpace(args.Persona))
	body := strings.TrimSpace(args.Body)

	// Validate scope.
	if scope != "bersama" && scope != "warga" {
		return Result{}, fmt.Errorf("plan_write: scope harus 'bersama' atau 'warga'")
	}
	// Validate period.
	if period != "yearly" && period != "monthly" && period != "daily" {
		return Result{}, fmt.Errorf("plan_write: period harus 'yearly'|'monthly'|'daily'")
	}
	// Validate persona.
	if scope == "bersama" {
		if persona != "" && persona != "wangsit" {
			return Result{}, fmt.Errorf("plan_write: scope=bersama HANYA boleh persona=wangsit (arsitek per ADR-016)")
		}
		persona = "wangsit" // implicit
	} else { // warga
		if persona == "" {
			return Result{}, fmt.Errorf("plan_write: scope=warga butuh persona")
		}
		if !validPlanPersonas[persona] {
			return Result{}, fmt.Errorf("plan_write: persona %q tidak dikenal (17 warga: lihat ADR-013)", persona)
		}
	}
	// Status required untuk daily.
	if period == "daily" && scope == "warga" && args.Status == "" {
		return Result{}, fmt.Errorf("plan_write: period=daily butuh status (DONE|WIP|BLOCKED)")
	}
	if args.Status != "" && args.Status != "DONE" && args.Status != "WIP" && args.Status != "BLOCKED" {
		return Result{}, fmt.Errorf("plan_write: status harus DONE|WIP|BLOCKED")
	}
	if body == "" {
		return Result{}, fmt.Errorf("plan_write: body kosong")
	}
	if len(body) > 100_000 {
		return Result{}, fmt.Errorf("plan_write: body terlalu besar (max 100KB)")
	}

	// Date resolver.
	now := time.Now().UTC()
	date := strings.TrimSpace(args.Date)
	if date == "" {
		switch period {
		case "yearly":
			date = now.Format("2006")
		case "monthly":
			date = now.Format("2006-01")
		case "daily":
			date = now.Format("2006-01-02")
		}
	}

	// Target path.
	var relPath string
	if scope == "bersama" {
		relPath = filepath.Join("docs", "plan", "bersama", period+".md")
	} else {
		relPath = filepath.Join("docs", "plan", "warga", persona, period+".md")
	}
	fullPath := filepath.Join(t.workspace, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return Result{}, fmt.Errorf("plan_write: mkdir: %w", err)
	}

	// Derive from monthly (daily only).
	if args.DeriveFrom == "monthly" && period == "daily" && scope == "warga" {
		monthlyPath := filepath.Join(t.workspace, "docs", "plan", "warga", persona, "monthly.md")
		if b, err := os.ReadFile(monthlyPath); err == nil {
			body = "> _Derived from monthly.md on " + date + "._\n\n" +
				"## Target hari ini (dari monthly milestones)\n\n" +
				extractMonthlyMilestones(string(b)) + "\n\n" +
				"---\n\n" + body
		}
	}

	// Build frontmatter + content.
	var sb strings.Builder
	sb.WriteString("---\n")
	if scope == "bersama" {
		sb.WriteString("persona: bersama\n")
		sb.WriteString("author: wangsit\n")
	} else {
		sb.WriteString(fmt.Sprintf("persona: %s\n", persona))
	}
	sb.WriteString(fmt.Sprintf("period: %s\n", period))
	sb.WriteString(fmt.Sprintf("date: %s\n", date))
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", now.Format(time.RFC3339)))
	if args.Status != "" {
		sb.WriteString(fmt.Sprintf("status: %s\n", args.Status))
	}
	if len(args.AlignedTo) > 0 {
		sb.WriteString("aligned_to:\n")
		for _, ref := range args.AlignedTo {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				sb.WriteString(fmt.Sprintf("  - %s\n", ref))
			}
		}
	}
	sb.WriteString("---\n\n")
	sb.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}

	// BUG-046 fix (2026-04-25): SafeWriteFile routes path through fsutil
	// traversal guard. Was tmp+rename bare — we lose the explicit atomic
	// rename here but SafeWriteFile is consistent with the rest of the
	// codebase and the path is already validated by the caller.
	if err := fsutil.SafeWriteFile(fullPath, []byte(sb.String()), 0644); err != nil {
		return Result{}, fmt.Errorf("plan_write: write: %w", err)
	}

	return Result{
		Output: fmt.Sprintf("Roadmap ditulis → %s (scope=%s period=%s persona=%s date=%s)",
			relPath, scope, period, persona, date),
		Metadata: map[string]any{
			"path":    relPath,
			"scope":   scope,
			"period":  period,
			"persona": persona,
			"date":    date,
			"status":  args.Status,
		},
	}, nil
}

// extractMonthlyMilestones scrape heading `## M<N>.` dari monthly.md buat
// scaffold daily derive. Kalau gak ada, return placeholder.
func extractMonthlyMilestones(content string) string {
	var items []string
	lines := strings.Split(content, "\n")
	for i, ln := range lines {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "### M") || strings.HasPrefix(trim, "## M") {
			// Take heading + next DoD line if any.
			items = append(items, "- [ ] "+strings.TrimLeft(trim, "# "))
			for j := i + 1; j < i+5 && j < len(lines); j++ {
				if strings.Contains(lines[j], "DoD") || strings.Contains(lines[j], "Target") {
					items = append(items, "  "+strings.TrimSpace(lines[j]))
				}
			}
		}
	}
	if len(items) == 0 {
		return "- [ ] (monthly.md belum ada milestone — isi manual dulu via plan_write period=monthly)"
	}
	return strings.Join(items, "\n")
}

// PlanReadTool — baca roadmap dari docs/plan/.
type PlanReadTool struct {
	workspace string
}

type planReadArgs struct {
	Scope   string `json:"scope,omitempty"`   // "bersama"|"warga"|"" (both)
	Persona string `json:"persona,omitempty"` // filter warga tertentu
	Period  string `json:"period,omitempty"`  // "yearly"|"monthly"|"daily"|"" (all)
	Full    bool   `json:"full,omitempty"`    // default false = preview 1KB
}

func NewPlanReadTool(workspace string) *PlanReadTool {
	return &PlanReadTool{workspace: workspace}
}

func (t *PlanReadTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "plan_read",
		Description: `Baca roadmap dari docs/plan/ (ADR-016).

Filter optional:
  scope:   "bersama" | "warga" | "" (default both)
  persona: filter warga tertentu (misal "aksara")
  period:  "yearly" | "monthly" | "daily" | "" (default all)
  full:    true = include body penuh. Default false = preview 1KB.

Semua warga bisa baca apapun (transparency).

Return: list entry dengan path/persona/period/date/status/preview-atau-body.`,
		// 2026-05-07 (Merpati keluh-kesah Telegram via Ayah): Gemini API
		// 400 INVALID_ARGUMENT kalau enum array contain empty string value
		// (function_declarations[N].parameters.properties.X.enum[K]: cannot
		// be empty). Empty string sebelumnya intent=optional/all, tapi
		// Gemini strict reject. Solusi: hapus "" dari enum — field tetap
		// optional via 'required' absent. Empty value treated as "all" di
		// handler logic (lihat plan_read.go).
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope":   map[string]any{"type": "string", "enum": []string{"bersama", "warga"}},
				"persona": map[string]any{"type": "string"},
				"period":  map[string]any{"type": "string", "enum": []string{"yearly", "monthly", "daily"}},
				"full":    map[string]any{"type": "boolean"},
			},
		},
	}
}

type planEntry struct {
	Path    string
	Scope   string
	Persona string
	Period  string
	Body    string
	ModTime time.Time
}

func (t *PlanReadTool) Execute(_ context.Context, inv Invocation) (Result, error) {
	var args planReadArgs
	if err := json.Unmarshal(inv.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("plan_read: invalid args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }


	planRoot := filepath.Join(t.workspace, "docs", "plan")
	filterPersona := sanitizePersonaName(strings.TrimSpace(args.Persona))
	filterPeriod := strings.ToLower(strings.TrimSpace(args.Period))
	filterScope := strings.ToLower(strings.TrimSpace(args.Scope))

	var entries []planEntry

	// Scan bersama/
	if filterScope == "" || filterScope == "bersama" {
		bersamaDir := filepath.Join(planRoot, "bersama")
		if items, err := os.ReadDir(bersamaDir); err == nil {
			for _, it := range items {
				if it.IsDir() || !strings.HasSuffix(it.Name(), ".md") {
					continue
				}
				period := strings.TrimSuffix(it.Name(), ".md")
				if filterPeriod != "" && period != filterPeriod {
					continue
				}
				path := filepath.Join(bersamaDir, it.Name())
				if b, err := os.ReadFile(path); err == nil {
					info, _ := it.Info()
					entries = append(entries, planEntry{
						Path: filepath.Join("bersama", it.Name()), Scope: "bersama",
						Persona: "bersama", Period: period,
						Body: string(b), ModTime: info.ModTime(),
					})
				}
			}
		}
	}

	// Scan warga/
	if filterScope == "" || filterScope == "warga" {
		wargaDir := filepath.Join(planRoot, "warga")
		if personaDirs, err := os.ReadDir(wargaDir); err == nil {
			for _, pd := range personaDirs {
				if !pd.IsDir() {
					continue
				}
				persona := pd.Name()
				if filterPersona != "" && !strings.EqualFold(persona, filterPersona) {
					continue
				}
				subDir := filepath.Join(wargaDir, persona)
				if items, err := os.ReadDir(subDir); err == nil {
					for _, it := range items {
						if it.IsDir() || !strings.HasSuffix(it.Name(), ".md") {
							continue
						}
						period := strings.TrimSuffix(it.Name(), ".md")
						if filterPeriod != "" && period != filterPeriod {
							continue
						}
						path := filepath.Join(subDir, it.Name())
						if b, err := os.ReadFile(path); err == nil {
							info, _ := it.Info()
							entries = append(entries, planEntry{
								Path:  filepath.Join("warga", persona, it.Name()),
								Scope: "warga", Persona: persona, Period: period,
								Body: string(b), ModTime: info.ModTime(),
							})
						}
					}
				}
			}
		}
	}

	if len(entries) == 0 {
		return Result{Output: "(belum ada roadmap match filter)"}, nil
	}

	// Sort: bersama dulu, terus per persona alphabetic, per period yearly>monthly>daily.
	periodOrder := map[string]int{"yearly": 0, "monthly": 1, "daily": 2}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Scope != entries[j].Scope {
			return entries[i].Scope == "bersama"
		}
		if entries[i].Persona != entries[j].Persona {
			return entries[i].Persona < entries[j].Persona
		}
		return periodOrder[entries[i].Period] < periodOrder[entries[j].Period]
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== %d roadmap entry ===\n\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("▸ **%s** · persona=%s period=%s · updated %s\n",
			e.Path, e.Persona, e.Period, e.ModTime.Format("2006-01-02 15:04 UTC")))
		body := e.Body
		if !args.Full && len(body) > 1024 {
			body = body[:1024] + "\n…[truncated — panggil dgn full=true untuk baca lengkap]"
		}
		sb.WriteString(body)
		sb.WriteString("\n\n---\n\n")
	}
	return Result{Output: sb.String()}, nil
}
