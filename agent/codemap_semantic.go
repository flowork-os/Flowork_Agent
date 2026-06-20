// codemap_semantic.go — R6 SELF-MAP SEMANTIK (sisi main: implementasi Summarizer).
// Owner-approved 2026-06-15 (FASE 2 autonomi). Nge-WIRE LLM (routerChat) ke enrich engine
// di agentmgr.CodemapEnrichHandler. Prompt FOKUS & kecil (prinsip semut): 1 file → JSON
// {summary,domain,role}. Decoupling: agentmgr gak tahu LLM, main inject lewat closure ini.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

// enricherModel — model GUI per-agent codemap-enricher (kv router_model). Buat label/laporan.
// Path .fwagent bener (pola sama evoCoderModel). Fallback coderModel kalau belum di-set.
func enricherModel() string {
	dir := filepath.Join(loader.AgentsDir(), enricherID+".fwagent")
	if st, e := agentdb.Open(agentdb.Resolve(enricherID, dir)); e == nil {
		defer st.Close()
		if m := st.GetRouterModel(); m != "" {
			return m
		}
	}
	return coderModel("")
}

func parseEnrich(raw string) (summary, domain, role string, ok bool) {
	var p struct {
		Summary string `json:"summary"`
		Domain  string `json:"domain"`
		Role    string `json:"role"`
	}
	if json.Unmarshal([]byte(jsonSlice(raw)), &p) != nil {
		return "", "", "", false
	}
	return strings.TrimSpace(p.Summary), strings.TrimSpace(p.Domain), strings.TrimSpace(p.Role),
		p.Summary != "" || p.Domain != "" || p.Role != ""
}

// LOCKED (soft, owner-approved 2026-06-20): enrich via agent codemap-enricher (model GUI). Jangan ubah tanpa izin.
// codemapSemanticSummarizer — implementasi agentmgr.SemanticSummarizer. Owner 2026-06-20: enrich
// jalan lewat AGENT codemap-enricher (model dari GUI, BUKAN hardcode = cacat arsitektur). Host
// KIRIM isi file → agent (persona cartographer di DB) balas JSON → parse. Fallback routerChat
// kalau agent ga ada / output ga valid (robust, ga mecahin enrich).
func codemapSemanticSummarizer(host *kernelhost.Host) agentmgr.SemanticSummarizer {
	return func(ctx context.Context, path, content, model string) (summary, domain, role, usedModel string, err error) {
		// 1) AGENT codemap-enricher (otak di agent, model GUI). System-prompt = persona DB-nya.
		if host != nil {
			if raw, e := host.InvokeAgentMessage(ctx, enricherID, "FILE: "+path+"\n\n"+content, "codemap-enrich"); e == nil {
				if s, d, r, ok := parseEnrich(extractReply(raw)); ok {
					return s, d, r, enricherModel() + " (codemap-enricher)", nil
				}
			}
		}
		// 2) Fallback: routerChat hardcoded (kalau agent belum ke-load / output invalid).
		usedModel = coderModel(model)
		sys := "You are a codebase cartographer. Given ONE source file, reply ONLY a compact JSON object: " +
			`{"summary":"one sentence: what this file does","domain":"functional area (e.g. auth, triggers, brain, ui, codemap, finance, router, orchestrator)","role":"architecture role (e.g. http-handler, engine, data-store, config, parser, wasm-agent, test)"}. ` +
			"No markdown, no code fences, no prose — JSON only."
		msgs := []map[string]any{
			{"role": "system", "content": sys},
			{"role": "user", "content": "FILE: " + path + "\n\n" + content},
		}
		res, e := routerChatSafe(ctx, usedModel, msgs, nil, 300)
		if e != nil {
			return "", "", "", usedModel, e
		}
		s, d, r, ok := parseEnrich(res.Content)
		if !ok {
			return "", "", "", usedModel, fmt.Errorf("bad json from model: %s", trimStr(res.Content, 80))
		}
		return s, d, r, usedModel + " (fallback)", nil
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
