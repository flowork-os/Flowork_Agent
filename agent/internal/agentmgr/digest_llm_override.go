// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-21 (AI-IN-AGENT mandate).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner + re-lock + changelog).
package agentmgr

// digest_llm_override.go — AI-IN-AGENT (owner 2026-06-21 KERAS: "semua AI WAJIB di
// agent, model GUI, bukan host+global/hardcode"). Dream/digest dulu manggil model
// GLOBAL (DefaultModelShared) langsung dari host (cognitive_digest_cron.go). Sekarang
// reasoning-nya dialihkan ke AGENT `dream-digester` (persona extractor + model GUI
// per-agent), pola SAMA codemap-enricher.
//
// Hook ini di-SET oleh main package saat boot (dreamDigestLLM lewat InvokeAgentMessage).
// nil = belum di-wire → buildDigestDeps fallback ke router langsung (robust, ga mecahin
// digest). Layering bersih: agentmgr expose hook, main package (yang pegang *Host) isi.

import "context"

// DigestLLMOverride — reasoning extraction digest via AGENT (model GUI). nil = fallback
// router langsung (DefaultModelShared). Di-set di main.go: agentmgr.DigestLLMOverride = ...
var DigestLLMOverride func(ctx context.Context, prompt string) (string, error)
