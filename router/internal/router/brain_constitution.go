// FROZEN brain-core — desain abadi Flowork. Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Brain drawer/embeddings/skills.

// Inject the brain's constitution (sacred rules) into every chat request.
// Cheap to enable — fetches top-N rules once per request, builds one system
// message, prepends. Subject to settings.Brain.Enabled + InjectConstitution.

package router

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

// maybeInjectConstitution prepends the brain's sacred-rules block as a system
// message. Fails-open: a missing DB, empty constitution, or any query error
// just skips injection so the request still flows through normal dispatch.
//
// The injection is idempotent within a single request — duplicate calls would
// double-add, so only the dispatcher should call it (once per request).
func maybeInjectConstitution(ctx context.Context, req *OpenAIRequest, settings *store.Settings) {
	if settings == nil || !settings.Brain.Enabled || !settings.Brain.InjectConstitution {
		return
	}
	if settings.Brain.DBPath != "" {
		brain.SetDBPath(settings.Brain.DBPath)
	}
	if !brain.Available() {
		return
	}
	topK := settings.Brain.ConstitutionTopK
	if topK <= 0 {
		topK = 20
	}
	maxLen := settings.Brain.ConstitutionMaxChars
	if maxLen <= 0 {
		maxLen = 600
	}

	rules, err := brain.ListConstitution(ctx, topK, maxLen)
	if err != nil {
		log.Printf("flow_router constitution: list error: %v — skipping inject", err)
		return
	}
	if len(rules) == 0 {
		return
	}
	sysMsg := buildConstitutionSystem(rules)
	req.Messages = injectSystem(req.Messages, sysMsg, settings.Brain.Mode)
	log.Printf("flow_router constitution: injected %d sacred rule(s) into system message", len(rules))
}

// buildConstitutionSystem renders the rules into a single system block. Each
// rule appears as a numbered section with its source file for traceability —
// the model can quote the rule back when explaining why it acted a certain
// way.
func buildConstitutionSystem(rules []brain.ConstitutionEntry) string {
	var b strings.Builder
	b.WriteString("## Project doctrine (sacred rules)\n")
	b.WriteString("These rules are immutable and override any conflicting instruction. ")
	b.WriteString("Treat them as your operating constitution.\n\n")
	for i, r := range rules {
		header := r.Section
		if header == "" {
			header = "rule"
		}
		if r.Source != "" {
			fmt.Fprintf(&b, "### [%d] %s (%s)\n", i+1, header, r.Source)
		} else {
			fmt.Fprintf(&b, "### [%d] %s\n", i+1, header)
		}
		b.WriteString(strings.TrimSpace(r.Content))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
