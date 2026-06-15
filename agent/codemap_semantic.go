// codemap_semantic.go — R6 SELF-MAP SEMANTIK (sisi main: implementasi Summarizer).
// Owner-approved 2026-06-15 (FASE 2 autonomi). Nge-WIRE LLM (routerChat) ke enrich engine
// di agentmgr.CodemapEnrichHandler. Prompt FOKUS & kecil (prinsip semut): 1 file → JSON
// {summary,domain,role}. Decoupling: agentmgr gak tahu LLM, main inject lewat closure ini.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"flowork-gui/internal/agentmgr"
)

// codemapSemanticSummarizer — implementasi agentmgr.SemanticSummarizer pakai routerChat.
func codemapSemanticSummarizer() agentmgr.SemanticSummarizer {
	return func(ctx context.Context, path, content, model string) (summary, domain, role, usedModel string, err error) {
		usedModel = coderModel(model) // resolve: arg → FLOWORK_CODER_MODEL → FLOWORK_LLM_MODEL → default
		sys := "You are a codebase cartographer. Given ONE source file, reply ONLY a compact JSON object: " +
			`{"summary":"one sentence: what this file does","domain":"functional area (e.g. auth, triggers, brain, ui, codemap, finance, router, orchestrator)","role":"architecture role (e.g. http-handler, engine, data-store, config, parser, wasm-agent, test)"}. ` +
			"No markdown, no code fences, no prose — JSON only."
		msgs := []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "FILE: " + path + "\n\n" + content},
		}
		res, e := routerChat(ctx, usedModel, msgs, nil, 300)
		if e != nil {
			return "", "", "", usedModel, e
		}
		var parsed struct {
			Summary string `json:"summary"`
			Domain  string `json:"domain"`
			Role    string `json:"role"`
		}
		if jerr := json.Unmarshal([]byte(jsonSlice(res.Content)), &parsed); jerr != nil {
			return "", "", "", usedModel, fmt.Errorf("bad json from model: %s", trimStr(res.Content, 80))
		}
		return strings.TrimSpace(parsed.Summary), strings.TrimSpace(parsed.Domain), strings.TrimSpace(parsed.Role), usedModel, nil
	}
}

// jsonSlice — ambil {…} pertama..terakhir dari output LLM (buang fence/prosa kalau ada).
func jsonSlice(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}
