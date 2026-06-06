// agent-config — CLI set persona + tool subscription 1 agent langsung ke
// state.db-nya (tanpa lewat GUI/auth). Dipakai setup crew reproducible
// (scripts/setup-saham-crew.sh) + provisioning headless.
//
// Pakai:
//
//	agent-config <state.db path> <persona> [tool1,tool2,...]
//
// Contoh:
//
//	agent-config agents/saham-teknikal/workspace/state.db "Lo analis teknikal..." web_search,html_extract
//
// state.db di-CREATE kalau belum ada (agentdb.Open). Persona masuk config
// (key "prompt" → FLOWORK_AGENT_CONFIG saat boot). Tools di-subscribe (source
// "manual") supaya ke-expose ke LLM via /api/agents/tools/specs.
package main

import (
	"fmt"
	"os"
	"strings"

	"flowork-gui/internal/agentdb"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: agent-config <state.db> <persona> [tool1,tool2,...]")
		os.Exit(2)
	}
	dbPath := os.Args[1]
	persona := os.Args[2]
	tools := ""
	if len(os.Args) >= 4 {
		tools = os.Args[3]
	}

	s, err := agentdb.Open(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer s.Close()

	// persona "-" = jangan ubah persona (subscribe-only mode).
	if persona != "-" {
		if err := s.Save(map[string]any{"prompt": persona}); err != nil {
			fmt.Fprintln(os.Stderr, "save persona:", err)
			os.Exit(1)
		}
	}
	subbed := 0
	for _, t := range strings.Split(tools, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if err := s.SubscribeTool(t, "manual", "{}"); err != nil {
			fmt.Fprintf(os.Stderr, "subscribe %s: %v\n", t, err)
			continue
		}
		subbed++
	}
	fmt.Printf("OK %s — persona set (%d char), %d tools subscribed\n", dbPath, len(persona), subbed)
}
