// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-16 (LOCKED ≠ FREEZE). AI lain:
// JANGAN otak-atik tanpa izin owner. Sisi main self-evolution: proposer LLM + evolveGateDeps
// (edition dev/public, ModelStrong=capabilityMeetsBar). Prompt KIND-RULES (kode≠add-skill) + LARANG
// delete/LOCKED. Wire ke agentmgr (loop autonomy di sana).
// 2026-06-21 (owner-approved, AI-IN-AGENT): proposer model coderModel("") (global=flowork-brain)
//   → evoCoderModel() (Opus per-agent GUI). Kebenaran model = setting per-agent evo-coder. Re-locked.
//
// selfevolve.go — R7 SELF-EVOLUTION fase-1 (sisi main: proposer LLM). Owner-approved
// 2026-06-15 (FASE 2 autonomi). Wire routerChat ke agentmgr.EvolveReflectHandler:
// kasih self-map semantik (R6) → LLM usulin perbaikan ADDITIVE & AMAN. FASE-1 = usulan
// doang (nol ubah kode). Prompt nge-LARANG delete / sentuh file LOCKED (pelajaran zombie:
// jangan asal, verifikasi dulu). Eksekusi auto-commit = fase-2 di-gate karma.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/floworkdb"
)

// evolveEvalHandler — POST /api/evolve/eval. Jalanin capability eval (model aktif disuruh
// nulis kode Go → compile+run deterministik; bar kalibrasi kelas Opus 4.7, NO hardcode nama)
// → cache. On-demand (tombol GUI), bukan tiap status-poll (eval ~90s). Owner-loopback.
func evolveEvalHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		res := evolveEvalAndCache()
		tfWriteJSON(w, 0, map[string]any{
			"ok": true, "model": res.Model, "passed": res.Passed,
			"score": res.Score, "total": res.Total, "detail": res.Detail,
		})
	}
}

// evolveGateDeps — rakit dependency gate (KV toggle + guard model) buat handler config.
func evolveGateDeps() agentmgr.EvolveGateDeps {
	return agentmgr.EvolveGateDeps{
		KVGet: func(k string) (string, error) {
			db, e := floworkdb.Shared()
			if e != nil {
				return "", e
			}
			return db.GetKV(k)
		},
		KVSet: func(k, v string) error {
			db, e := floworkdb.Shared()
			if e != nil {
				return e
			}
			return db.SetKV(k, v)
		},
		ModelStrong: capabilityMeetsBar, // eval-based (no hardcode nama), cache-only di status
		Edition: func() string {
			// FLOWORK_EDITION=dev → evolusi penuh (core+behavior). Default public = aman
			// (behavior-layer aja, core via auto-update). make-distributable set ini per profil.
			if strings.EqualFold(strings.TrimSpace(os.Getenv("FLOWORK_EDITION")), "dev") {
				return "dev"
			}
			return "public"
		},
	}
}

func evolveProposer() agentmgr.EvolveProposer {
	return func(ctx context.Context, selfMapContext, focus string) ([]agentmgr.ProposalDraft, error) {
		// AI-IN-AGENT (owner 2026-06-21): kebenaran model = setting PER-AGENT. Proposer
		// evolusi pakai model AGENT evo-coder (evoCoderModel = Opus GUI), BUKAN coderModel("")
		// = global (yg ternyata flowork-brain → proposal jadi lemah). Fix komentar header.
		model := evoCoderModel()
		foc := strings.TrimSpace(focus)
		if foc == "" {
			foc = "perbaikan yang naikin autonomi, ketahanan (resilience), atau ngisi celah kemampuan"
		}
		sys := "You are Flowork's self-evolution architect. You receive a SEMANTIC SELF-MAP of the codebase " +
			"(lines: 'path [domain/role]: summary'). Propose 3-5 CONCRETE, SAFE, ADDITIVE improvements. " +
			`Reply ONLY a JSON array: [{"target_file":"path (or NEW:path for new file)","kind":"add-agent|add-skill|add-app|fix|refactor|doc|test","rationale":"1-2 sentences: what + why","risk":"low|medium|high"}]. ` +
			"KIND RULES (match kind to target, do NOT mix): " +
			"• add-skill = a reusable SKILL.md (a procedure/knowledge for agents to follow). target_file MUST be 'NEW:<skill-name>' — a short name, NEVER a code path. " +
			"• add-agent / add-app = a whole new agent or app (behavior artifact). " +
			"• fix | refactor | test | doc = a change to REPO SOURCE (a real *.go / *.md path = the actual target_file). " +
			"A *.go or any real repo file = fix/refactor/test/doc — NEVER add-skill. " +
			"PREFER behavior-layer (add-skill / add-agent / add-app): those APPLY directly here. Repo-source kinds (fix/refactor/test/doc) only land via upstream in the public edition, so propose them sparingly. " +
			"NEVER propose deleting files or editing files marked LOCKED. No prose, JSON array only."
		user := "FOCUS: " + foc + "\n\nSELF-MAP (semantik):\n" + selfMapContext
		res, e := routerChatSafe(ctx, model, []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": user},
		}, nil, 1400)
		if e != nil {
			return nil, e
		}
		var arr []agentmgr.ProposalDraft
		if jerr := json.Unmarshal([]byte(jsonArraySlice(res.Content)), &arr); jerr != nil {
			return nil, fmt.Errorf("bad json from model: %s", trimStr(res.Content, 100))
		}
		for i := range arr {
			arr[i].Model = model
		}
		return arr, nil
	}
}

// jsonArraySlice — ambil [...] pertama..terakhir dari output LLM (buang fence/prosa).
func jsonArraySlice(s string) string {
	i := strings.Index(s, "[")
	j := strings.LastIndex(s, "]")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}
