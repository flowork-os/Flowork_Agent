// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-21 (AI-IN-AGENT mandate).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner + re-lock + changelog).
package main

// dream_digester_seed.go — AGENT untuk CGM dream/digest (owner 2026-06-21 KERAS:
// "SEMUA AI WAJIB di agent, model GUI, BUKAN host+global = cacat arsitektur"). Dulu
// digest extraction = LLM model GLOBAL (DefaultModelShared) langsung di host
// (cognitive_digest_cron.go). Sekarang otaknya pindah ke agent `dream-digester`:
// persona extractor di DB, model dari Settings per-agent (kv router_model). Host cuma
// KIRIM prompt extraction → agent balas output → parse. Idempotent. Pola PERSIS
// codemap-enricher / ai-studio (otak di agent, model GUI, host orkestrasi).

import (
	"context"
	"os"
	"path/filepath"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

const digesterID = "dream-digester"

// digesterPersona — persona MINIMAL: prompt extraction (instruksi + skema lengkap)
// dikirim host sebagai pesan user, jadi persona cuma negasin biar output BERSIH (JSON
// doang, no prosa). Bisa di-tweak owner di GUI. Model di-set owner di Settings (GUI).
const digesterPersona = "Kamu CGM EXTRACTOR Flowork. Pesan user berisi instruksi + skema LENGKAP " +
	"buat ekstrak entitas/relasi (node & edge) dari teks. IKUTI instruksi & skema itu PERSIS. " +
	"Balas HANYA output yang diminta (JSON sesuai skema) — tanpa markdown, tanpa code-fence, " +
	"tanpa prosa/penjelasan/sapaan. Output only."

// seedDreamDigester — bikin agent dream-digester (idempoten). Model TIDAK di-hardcode
// (owner set di Settings GUI). Dipanggil boot SETELAH template wasm ke-build, deket
// seedCodemapEnricher().
func seedDreamDigester() {
	agentsDir := loader.AgentsDir()
	dir := filepath.Join(agentsDir, digesterID+".fwagent")
	if _, e := os.Stat(dir); e != nil {
		tplWasm, err := os.ReadFile(filepath.Join("templates", "agent-template", "agent.wasm"))
		if err != nil || len(tplWasm) == 0 {
			return // template belum ke-build → skip (start.sh build dulu)
		}
		if mk := os.MkdirAll(filepath.Join(dir, "workspace"), 0o755); mk != nil {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "manifest.json"), evoMemberManifest(digesterID, "Dream Digester"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "agent.wasm"), tplWasm, 0o644)
		_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("agent.wasm\nworkspace/*.db\nworkspace/*.db-*\n"), 0o644)
	}
	// persona extractor; TANPA tool (pure extraction, ga butuh tool-loop) → output bersih.
	if st, e := agentdb.Open(agentdb.Resolve(digesterID, dir)); e == nil {
		_ = st.SetPrompt(digesterPersona)
		// Default model claude-haiku-4-5 (owner 2026-06-21: "agent baru pake haiku saja").
		// HANYA set kalau owner belum pilih di GUI → owner tetap bisa ganti kapan aja.
		if st.GetRouterModel() == "" {
			_ = st.SetRouterModel("claude-haiku-4-5")
		}
		_ = st.Close()
	}
	agentmgr.ProvisionAgentDNA(digesterID) // warga penuh (konstitusi sacred + DNA)
}

// dreamDigestLLM — reasoning extraction digest via AGENT dream-digester (model GUI).
// Di-wire ke agentmgr.DigestLLMOverride di main. Balikin err → buildDigestDeps fallback
// ke router langsung (robust).
func dreamDigestLLM(host *kernelhost.Host) func(context.Context, string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		raw, err := host.InvokeAgentMessage(ctx, digesterID, prompt, "cgm-digest")
		if err != nil {
			return "", err
		}
		return extractReply(raw), nil
	}
}
