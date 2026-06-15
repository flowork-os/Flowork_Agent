// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked at: 2026-06-15 (owner-approved autonomous sprint)
// Reason: Flowork Architect — group/team creator. VERIFIED E2E: POST /api/architect/build
//   {"prompt":"team peramal …"} → designed "Tim Peramal" (4 specialists primbon/zodiak/
//   fengshui/kalender + lead) → installed 11 agents → created group "peramal" (group.json +
//   loket group=1 + SyncToOrchestrator) → coordinator loaded → /api/chat {"agent":"peramal"}
//   returned a real holistic fortune (fan-out + synth). One LLM call (design) + fast local
//   assembly. Loopback-only (allowlist in floworkauth/handlers.go), owner trust = /api/coder/*.
//
// architect.go — FLOWORK ARCHITECT: stand up a whole TEAM (group) from one natural
// prompt. "buatin team peramal" → ONE structured design call (Opus, forced tool)
// returns the full roster (every specialist's persona/directive + a lead) → the Go
// engine deterministically assembles + installs each agent with the SAME proven
// machinery that built the saham/crypto/primbon crews (coderAssemblePack →
// installPluginPack) → groupsapi.CreateGroup wires them into a coordinator group.
// Result: the team shows up in the GUI Group tab, is chattable via
// POST /api/chat {"agent":"<group_id>"} (the coordinator fans out to members and
// synthesizes), and its Telegram slash command auto-registers.
//
//	POST /api/architect/build {prompt|task, model?}  → design team → build agents → create group
//
// ONE LLM call (not N+2): a multi-agent team used to need a design call per member,
// and with a rate-limited upstream each call stalls ~90s on 429-retries → the whole
// build timed out. Designing the entire team in a single forced-tool call keeps the
// build to one upstream round-trip; everything after is fast local Go. Principle
// "agent bodoh, engine pinter": the LLM only fills the creative SPEC. Loopback-only,
// owner-gated (install auto-approves caps because this endpoint is reachable only
// from the trusted loopback — same trust model as /api/coder/*).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/groupsapi"
	"flowork-gui/internal/kernelhost"
)

// idReGroup is tighter than groupsapi's idRe (2-40): a group_id here also becomes
// the lead category "<group_id>-lead", which must satisfy coderCatRe (max 31). Cap
// the group_id at 26 chars so "<group_id>-lead" never overflows.
var idReGroup = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,25}$`)

// teamWorker — one specialist (worker) in the team, fully specified by the design
// call. Only worker-side fields; the synth-side fields of its AgentSpec are filled
// with defaults at assembly (each specialist contributes its -worker to the group).
type teamWorker struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	Role       string `json:"role"`
	Persona    string `json:"persona"`
	Directive  string `json:"directive"`
}

// teamLead — the synthesizer/lead that combines the workers' outputs.
type teamLead struct {
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Persona   string `json:"persona"`
	Directive string `json:"directive"`
}

// teamPlan — the Architect's complete design (one forced-tool call fills all of it).
type teamPlan struct {
	GroupID     string       `json:"group_id"`
	DisplayName string       `json:"display_name"`
	Task        string       `json:"task"`
	Specialists []teamWorker `json:"specialists"`
	Lead        teamLead     `json:"lead"`
}

// architectDesignTeam — one Opus forced-tool call → the full team design. tool_choice
// is forced (no free-text hallucination; same pattern as coderDesignSpec).
func architectDesignTeam(ctx context.Context, prompt, model string) (teamPlan, error) {
	var plan teamPlan
	workerItem := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category_id": map[string]any{"type": "string", "description": "id slug unik specialist, lowercase-dash, 2-31 char (mis. 'primbon-jawa', 'zodiak')."},
			"name":        map[string]any{"type": "string", "description": "nama specialist human-readable (mis. 'Ahli Primbon Jawa')."},
			"icon":        map[string]any{"type": "string", "description": "1 emoji yang cocok."},
			"role":        map[string]any{"type": "string", "description": "label peran singkat (mis. 'penafsir weton')."},
			"persona":     map[string]any{"type": "string", "description": "persona/system-prompt specialist ini (keahlian + gaya). RINGKAS."},
			"directive":   map[string]any{"type": "string", "description": "cara kerja specialist. Kalau KREATIF/tradisi (ga butuh data real) bilang itu tugasnya; kalau ANALISIS suruh cari data."},
		},
		"required": []string{"category_id", "name", "icon", "role", "persona", "directive"},
	}
	tool := map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "design_team",
			"description": "Rancang 1 TIM (group) Flowork LENGKAP dari permintaan user: 2-4 specialist (worker) yg saling melengkapi + 1 lead (synthesizer). Isi SEMUA field sekali jalan. WAJIB dipanggil sekali.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"group_id":     map[string]any{"type": "string", "description": "id slug unik group, lowercase-dash, 2-26 char (mis. 'peramal', 'tim-kuliner')."},
					"display_name": map[string]any{"type": "string", "description": "nama tim human-readable (mis. 'Tim Peramal')."},
					"task":         map[string]any{"type": "string", "description": "instruksi kerja BERSAMA tim: apa yg tim hasilkan + cara koordinasi. SINGKAT."},
					"specialists":  map[string]any{"type": "array", "description": "2-4 specialist (worker) yg saling melengkapi.", "minItems": 2, "maxItems": 4, "items": workerItem},
					"lead": map[string]any{
						"type":        "object",
						"description": "lead/synthesizer yg gabungin output para specialist jadi 1 jawaban final.",
						"properties": map[string]any{
							"name":      map[string]any{"type": "string", "description": "nama lead (mis. 'Peramal Utama')."},
							"icon":      map[string]any{"type": "string", "description": "1 emoji."},
							"persona":   map[string]any{"type": "string", "description": "persona/system-prompt lead (perakit jawaban final). RINGKAS."},
							"directive": map[string]any{"type": "string", "description": "format output final: struktur + gaya. SINGKAT."},
						},
						"required": []string{"name", "icon", "persona", "directive"},
					},
				},
				"required": []string{"group_id", "display_name", "task", "specialists", "lead"},
			},
		},
	}
	args, err := routerForcedTool(ctx, model,
		"Lo arsitek TIM Flowork. Dari permintaan user, rancang group LENGKAP sekali jalan: pecah jadi 2-4 specialist (worker) yg saling melengkapi + 1 lead yg gabungin jadi 1 jawaban. Persona & directive sesuai domain. Bahasa Indonesia. RINGKAS (anti over-prompt).",
		"Bikin tim buat: "+prompt, tool, "design_team", 2500)
	if err != nil {
		return plan, err
	}
	if err := json.Unmarshal(args, &plan); err != nil {
		return plan, fmt.Errorf("decode team plan: %w", err)
	}
	plan.GroupID = strings.ToLower(strings.TrimSpace(plan.GroupID))
	plan.DisplayName = strings.TrimSpace(plan.DisplayName)
	return plan, nil
}

// nonEmpty returns v trimmed, or def if v is blank — fills AgentSpec fields the
// design call legitimately leaves out (a specialist has no synth role, etc.) so
// AgentSpec.validate() passes without burdening the LLM with throwaway text.
func nonEmpty(v, def string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return def
}

// architectBuild — full pipeline from a single design call: design → assemble+install
// each specialist + the lead → create the coordinator group. All steps after the one
// LLM call are local Go (fast).
func architectBuild(ctx context.Context, host *kernelhost.Host, store *floworkdb.Store, groups *groupsapi.Handler, prompt, model string) (map[string]any, error) {
	plan, err := architectDesignTeam(ctx, prompt, model)
	if err != nil {
		return nil, fmt.Errorf("design team: %w", err)
	}
	if !idReGroup.MatchString(plan.GroupID) {
		return nil, fmt.Errorf("group_id invalid/too long (2-26 lowercase-dash): %q", plan.GroupID)
	}
	if plan.DisplayName == "" {
		plan.DisplayName = plan.GroupID
	}
	if len(plan.Specialists) == 0 {
		return nil, fmt.Errorf("plan has no specialists")
	}

	built := []map[string]any{}
	members := []string{}
	seen := map[string]bool{}
	for _, sp := range plan.Specialists {
		cat := strings.ToLower(strings.TrimSpace(sp.CategoryID))
		if !coderCatRe.MatchString(cat) || seen[cat] || cat == plan.GroupID {
			continue // skip invalid/duplicate ids and any clash with the group id
		}
		seen[cat] = true
		// Build a full AgentSpec from the worker design; synth-side fields are unused
		// (we take the -worker into the group) but must be non-empty for validate().
		spec := AgentSpec{
			CategoryID:      cat,
			Name:            nonEmpty(sp.Name, cat),
			Icon:            nonEmpty(sp.Icon, "🤖"),
			TriggerHint:     "anggota tim " + plan.DisplayName + " — " + nonEmpty(sp.Role, "specialist"),
			WorkerRole:      nonEmpty(sp.Role, "specialist"),
			WorkerPersona:   nonEmpty(sp.Persona, "Specialist "+plan.DisplayName+"."),
			WorkerDirective: nonEmpty(sp.Directive, "Kerjakan bagianmu sesuai keahlian, ringkas."),
			SynthPersona:    "Perakit jawaban (tidak dipakai sebagai member).",
			SynthDirective:  "Gabungkan ringkas.",
		}
		if msg := spec.validate(); msg != "" {
			return nil, fmt.Errorf("specialist %s spec invalid: %s", cat, msg)
		}
		pack, perr := coderAssemblePack(spec)
		if perr != nil {
			return nil, fmt.Errorf("assemble %s: %w", cat, perr)
		}
		if res := installPluginPack(host, store, pack, true); res.status != 0 {
			return nil, fmt.Errorf("install specialist %s failed: %v", cat, res.body)
		}
		members = append(members, cat+"-worker")
		built = append(built, map[string]any{"category": cat, "worker": cat + "-worker", "name": spec.Name})
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("no valid specialists were built")
	}

	// Lead/synthesizer — its own generated category; its -synth becomes the group
	// synthesizer (coderDesignSpec/assemble synth persona = "perakit output final").
	leadCat := plan.GroupID + "-lead"
	if !coderCatRe.MatchString(leadCat) {
		return nil, fmt.Errorf("lead category overflow: %q", leadCat)
	}
	leadSpec := AgentSpec{
		CategoryID:      leadCat,
		Name:            nonEmpty(plan.Lead.Name, plan.DisplayName+" — Lead"),
		Icon:            nonEmpty(plan.Lead.Icon, "🧭"),
		TriggerHint:     "sintesis jawaban final tim " + plan.DisplayName,
		WorkerRole:      "lead",
		WorkerPersona:   nonEmpty(plan.Lead.Persona, "Lead tim "+plan.DisplayName+"."),
		WorkerDirective: "Gabungkan output anggota jadi 1 jawaban utuh.",
		SynthPersona:    nonEmpty(plan.Lead.Persona, "Lead/synthesizer tim "+plan.DisplayName+"."),
		SynthDirective:  nonEmpty(plan.Lead.Directive, "Rangkai jadi 1 jawaban final yg jelas + rapi."),
	}
	if msg := leadSpec.validate(); msg != "" {
		return nil, fmt.Errorf("lead spec invalid: %s", msg)
	}
	leadPack, aerr := coderAssemblePack(leadSpec)
	if aerr != nil {
		return nil, fmt.Errorf("assemble lead: %w", aerr)
	}
	if res := installPluginPack(host, store, leadPack, true); res.status != 0 {
		return nil, fmt.Errorf("install lead failed: %v", res.body)
	}
	synthesizer := leadCat + "-synth"

	// Wire the coordinator group (folder + roster + orchestrator sync). Live now.
	if cerr := groups.CreateGroup(plan.GroupID, plan.DisplayName, members, synthesizer, plan.Task); cerr != nil {
		return nil, fmt.Errorf("create group: %w", cerr)
	}

	return map[string]any{
		"ok":           true,
		"group_id":     plan.GroupID,
		"display_name": plan.DisplayName,
		"task":         plan.Task,
		"members":      members,
		"synthesizer":  synthesizer,
		"specialists":  built,
		"chat":         fmt.Sprintf("POST /api/chat {\"agent\":%q,\"text\":\"...\"}", plan.GroupID),
		"next":         "Team is live in the Group tab + Telegram slash menu. Chat it via the group id above.",
	}, nil
}

// architectBuildHandler — POST /api/architect/build {prompt|task, model?}.
func architectBuildHandler(host *kernelhost.Host, store *floworkdb.Store, groups *groupsapi.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Prompt string `json:"prompt"`
			Task   string `json:"task"` // alias for prompt
			Model  string `json:"model"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
			return
		}
		prompt := strings.TrimSpace(body.Prompt)
		if prompt == "" {
			prompt = strings.TrimSpace(body.Task)
		}
		if prompt == "" {
			tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "prompt required"})
			return
		}
		// One design call (may stall ~90s if upstream rate-limits before failover) +
		// fast local assembly → generous but bounded timeout.
		ctx, cancel := context.WithTimeout(r.Context(), 280*time.Second)
		defer cancel()
		res, err := architectBuild(ctx, host, store, groups, prompt, coderModel(body.Model))
		if err != nil {
			tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		tfWriteJSON(w, 0, res)
	}
}
