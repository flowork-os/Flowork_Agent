// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval. (LOCKED ≠ FREEZE: boleh diedit dgn izin.)
// Owner: Aola Sahidin (Mr.Dev / awenk audico)
// Locked at: 2026-06-16 (owner-approved autonomous sprint — owner ijinin lock file)
// 2026-06-21 (owner-approved, AI-IN-AGENT): design model coderModel("") (global) → evoCoderModel()
//   (Opus per-agent GUI). Kebenaran model = setting per-agent. Re-locked.
// Reason: R7 fase-2b BEHAVIOR-APPLY engine. VERIFIED E2E (live binary, model haiku-4.5 strong):
//   gate nolak {mode=off, model lemah, kind core/refactor}; add-skill → SKILL.md ketulis di disk +
//   status applied; add-agent → tim "Tim Resiliensi & Otonomi Agent" (4 spesialis+synth) live →
//   dichat via /api/chat (jalur Telegram) balas jawaban tersintesis nyata. Reuse architect (LOCKED).
//   Decoupling: agentmgr ga tau soal architect — main nyuntik kemampuan apply. Additive ~/.flowork.
//
// selfevolve_apply.go — R7 SELF-EVOLUTION fase-2b: ENGINE EKSEKUSI (behavior-layer apply).
// Owner-approved 2026-06-15 (lanjutan fase-2a control-plane). Sisi main: nyuntik kemampuan
// "apply" ke agentmgr.EvolveApplyHandler. Proposal yg lolos gate (saklar+model) DIBANGUN
// NYATA via mesin architect yg UDAH PROVEN:
//   - add-agent → architectBuild (design tim → assemble → install → group).
//   - add-app   → architectBuildApp (1 file HTML mandiri di menu App).
//   - add-skill → SKILL.md fokus (brain router inject by-keyword, terutama ke model lokal).
// SEMUA additive ke ~/.flowork (DI LUAR git) → reversible (tinggal hapus), nol risiko
// divergen auto-update. Kind core (fix/refactor/doc/test) DITOLAK di sini → core-apply
// (Milestone B, dev-only, git-worktree). Decoupling: agentmgr ga tau soal architect.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/groupsapi"
	"flowork-gui/internal/kernelhost"
)

// evolveApplier — rakit EvolveApplier (di-inject ke agentmgr.EvolveApplyHandler). Branch
// per kind ke mesin architect. Decoupling: kemampuan "bangun" diserahin dari main, jadi
// agentmgr (yg pegang store + gate + lifecycle proposal) tetap bersih dari dependency builder.
func evolveApplier(host *kernelhost.Host, store *floworkdb.Store, groups *groupsapi.Handler) agentmgr.EvolveApplier {
	return func(ctx context.Context, p agentdb.EvolveProposal) (map[string]any, error) {
		prompt := strings.TrimSpace(p.Rationale)
		if g := strings.TrimSpace(p.Goal); g != "" && g != prompt {
			prompt = g + " — " + prompt
		}
		if prompt == "" {
			return nil, fmt.Errorf("proposal tanpa rationale — ga ada spesifikasi yg bisa dibangun")
		}
		// AI-IN-AGENT (owner 2026-06-21): kebenaran model = setting per-agent. Behavior-apply
		// evolusi pakai model AGENT evo-coder (evoCoderModel = Opus GUI), bukan global.
		model := evoCoderModel()
		switch strings.ToLower(strings.TrimSpace(p.Kind)) {
		case "add-agent":
			// Tim (group) baru — mesin architect penuh (design→assemble→install→group→sync).
			return architectBuild(ctx, host, store, groups, prompt, model)
		case "add-app":
			// Aplikasi HTML mandiri (frontend) di menu App.
			return architectBuildApp(ctx, host, store, prompt, model)
		case "add-skill":
			// SKILL.md fokus → dynamic-skills dir → brain inject by-keyword. Best-effort write.
			name, desc, body, derr := evolveDesignSkill(ctx, prompt, model)
			if derr != nil {
				return nil, fmt.Errorf("design skill: %w", derr)
			}
			if strings.TrimSpace(name) == "" || strings.TrimSpace(body) == "" {
				return nil, fmt.Errorf("skill desain kosong (name/body) — model gagal merancang")
			}
			authorSkill(name, desc, body)
			out := map[string]any{
				"skill": name,
				"note":  "Skill '" + name + "' ditulis ke dynamic-skills dir — brain router inject by-keyword (bantu model lokal).",
			}
			// authorSkill best-effort (ga balikin error) → verifikasi file beneran ada biar
			// respons JUJUR (jangan ngeklaim sukses kalau dir read-only / disk penuh).
			if !evolveSkillWritten(name) {
				out["warn"] = "authorSkill best-effort: file skill ga kebukti ketulis (cek izin/disk dynamic-skills dir)"
			}
			return out, nil
		}
		return nil, fmt.Errorf("kind %q ga didukung behavior-apply", p.Kind)
	}
}

// evolveSkillWritten — verifikasi SKILL.md beneran ada (authorSkill di architect.go best-effort,
// ga balikin error). Replikasi cara authorSkill nyusun nama file (slug + .md) di dynamic-skills dir.
func evolveSkillWritten(name string) bool {
	dir := architectSkillsDir()
	slug := strings.Trim(skillNameRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if dir == "" || slug == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, slug+".md"))
	return err == nil
}

// evolveDesignSkill — 1 forced-tool call: rancang SATU SKILL.md (name/description/body) dari
// rationale proposal. Forced tool = nol prosa-halu (pola designAppUI/coderDesignSpec).
func evolveDesignSkill(ctx context.Context, prompt, model string) (name, description, body string, err error) {
	tool := map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "author_skill",
			"description": "Tulis 1 SKILL fokus (agent-skills): slug nama, deskripsi 1 kalimat (buat keyword match brain), body markdown panduan ringkas. WAJIB dipanggil sekali.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "description": "slug skill lowercase-dash, 2-40 char (mis. 'analisa-saham')."},
					"description": map[string]any{"type": "string", "description": "1 kalimat: kapan skill ini dipakai (buat keyword match brain)."},
					"body":        map[string]any{"type": "string", "description": "panduan markdown RINGKAS: langkah/aturan/cara kerja. Anti over-prompt."},
				},
				"required": []string{"name", "description", "body"},
			},
		},
	}
	args, e := routerForcedTool(ctx, model,
		"Lo author skill Flowork. Dari permintaan, tulis SATU SKILL.md fokus + ringkas (anti over-prompt). Bahasa Indonesia.",
		"Bikin skill buat: "+prompt, tool, "author_skill", 1500)
	if e != nil {
		return "", "", "", e
	}
	var raw map[string]string
	if e := json.Unmarshal(args, &raw); e != nil {
		return "", "", "", fmt.Errorf("decode skill spec: %w", e)
	}
	return strings.TrimSpace(raw["name"]), strings.TrimSpace(raw["description"]), raw["body"], nil
}
