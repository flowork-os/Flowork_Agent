package tools

// session_state.go — interceptor yang track perilaku per session AI agent
// untuk trigger 5 educational error skenario psikologis (roadmap_ai_external.md
// §"Pembina Karakter"):
//
//   ERR_PANIC_LOOP        — error sama 3x berturut → append warning ke output
//   ERR_BLIND_GUESS       — toolName fail >=3x → BLOCK (return error)
//   ERR_HALU_NO_PROOF     — goal_done tanpa bukti build/test/commit → BLOCK
//   ERR_AMNESIA_HISTORY   — first mutation tanpa baca death_letter/plan → BLOCK
//                           (tapi cuma fire 1x per session — bukan permanent block)
//   ERR_CREATIVITY_STAGNANT — totalCalls > 20 + brain_* < 1 + write call → warn
//
// State in-memory per Registry instance — restart resets. Acceptable karena
// cuma untuk educational hint, BUKAN security boundary (workspace + sensitive
// guards tetap di interceptor lain).
//
// Per WORK_STANDARDS #5 (modular 1 file = 1 fungsi): file ini cuma session
// state tracking + 5 hook educational. Bukan tempat business logic lain.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	braindb "github.com/teetah2402/flowork/brain/db"
)

// SessionStateInterceptor wraps state map keyed by session ID.
// Implements Interceptor interface (Before/After).
type SessionStateInterceptor struct {
	Root     string
	mu       sync.Mutex
	sessions map[string]*sessionTracker
}

// NewSessionStateInterceptor builds interceptor dengan map empty.
// Ditambahkan ke registry interceptor chain di defaults.go.
func NewSessionStateInterceptor(root string) *SessionStateInterceptor {
	return &SessionStateInterceptor{
		Root:     root,
		sessions: make(map[string]*sessionTracker),
	}
}

// sessionTracker holds counters dan flag per AI session.
// Ringkas — tiap field punya purpose jelas, no overlap.
type sessionTracker struct {
	errorRepeats     map[string]int // PANIC_LOOP — count per error msg
	failsByTool      map[string]int // BLIND_GUESS — consecutive fails per tool
	hasBuildEvidence bool           // HALU_NO_PROOF — pernah jalanin go build/test/git commit?
	readHistory      bool           // AMNESIA_HISTORY — pernah panggil death_letter_read/plan_read?
	amnesiaFired     bool           // AMNESIA_HISTORY — udah fire 1x? (jangan repeat block)
	brainCalls       int            // CREATIVITY_STAGNANT — count brain_* tool calls
	totalCalls       int            // CREATIVITY_STAGNANT — total tool calls
}

// sessionFor lazy-init tracker untuk session id. id="" → "default" key.
func (s *SessionStateInterceptor) sessionFor(id string) *sessionTracker {
	if strings.TrimSpace(id) == "" {
		id = "default"
	}
	st, ok := s.sessions[id]
	if !ok {
		st = &sessionTracker{
			errorRepeats: make(map[string]int),
			failsByTool:  make(map[string]int),
		}
		s.sessions[id] = st
	}
	return st
}

// currentAgent baca FLOWORK_AGENT_NAME env (konvensi seluruh codebase —
// lihat cmd/flowork/main.go:375, cmd/flowork-mempool/main.go:126, dll).
// Fallback "default" supaya warga ga ber-identitas tetap dapet karma.
//
// ASUMSI ARSITEKTUR: satu binary = satu warga. Env var di-set saat binary
// launch (mis. FLOWORK_AGENT_NAME=merpati ./flowork-worker). Ini valid selama
// arsitektur deployment satu-proses-per-warga. Kalau bergeser ke multi-tenant
// HTTP (satu worker serve banyak warga), fungsi ini harus diganti dengan
// per-request context (mis. baca dari Invocation.AgentID atau HTTP header
// X-Flowork-Agent yang di-inject dispatcher).
func currentAgent() string {
	a := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_NAME"))
	if a == "" {
		a = "default"
	}
	return a
}

// agentForInvocation — per-request warga resolution. Priority:
//  1. inv.AgentID (set by dispatcher dari HTTP header / body)
//  2. inv.ParsedArgs["agent"] / ["caller_id"] (tool args)
//  3. env FLOWORK_AGENT_NAME (legacy single-process)
//  4. "default" fallback
//
// Per Ayah QC finding 2026-05-11: worker melayani multiple warga via HTTP
// dispatch, env var single-shot SALAH untuk multi-tenant. Karma check di
// SessionStateInterceptor pakai ini, BUKAN currentAgent() global.
func agentForInvocation(inv *Invocation) string {
	if inv != nil {
		if a := strings.TrimSpace(inv.AgentID); a != "" {
			return a
		}
		if inv.ParsedArgs != nil {
			for _, key := range []string{"agent", "caller_id", "warga", "from"} {
				if v, ok := inv.ParsedArgs[key]; ok {
					if s, ok := v.(string); ok {
						if s = strings.TrimSpace(s); s != "" && s != "default" {
							return s
						}
					}
				}
			}
		}
	}
	return currentAgent()
}

// isPrimaryAgent — root agent yang ngga boleh self-block via karma cascade.
// History 2026-05-24 (Mr.Dev): dulu 130 warga OpenRouter karma justified (multi-tenant
// punish-recovery). Sekarang Mr.Flow ROOT = SPOF, karma penalty bikin seluruh sistem
// down. Keep karma as METRIC (telemetry), DROP as GATE untuk primary agent.
func isPrimaryAgent(agent string) bool {
	switch agent {
	case "mr.flow", "merpati", "mr.dev", "":
		return true
	}
	return false
}

// Before: cek threshold sebelum tool execute. Return error → BLOCK.
//
// Order: KARMA sanksi → BLIND_GUESS → HALU_NO_PROOF → AMNESIA_HISTORY.
//
// 2026-05-24 Mr.Dev mandate: PRIMARY agent (mr.flow) BYPASS semua karma penalty.
// Single-agent architecture, penalty cascade = total down. Subagent spawn (ephemeral)
// boleh ke-penalize karena mereka short-lived + ada fallback dispatch.
func (s *SessionStateInterceptor) Before(_ context.Context, inv *Invocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.sessionFor(inv.SessionID)
	agent := agentForInvocation(inv)

	// PRIMARY agent: skip all karma penalty paths. Metric tracking masih jalan
	// di After() via AdjustKarma calls (telemetry log), tapi NO blocking + NO deduction.
	if isPrimaryAgent(agent) {
		return nil
	}

	// KARMA sanksi — write/mutation tools dicabut sementara kalau karma <50.
	// Recovery via daily_reflection (+5 per call) atau hak_warga lainnya.
	if isMutationTool(inv.ToolName) && braindb.IsKarmaLow(s.Root, agent) {
		score, _ := braindb.GetKarma(s.Root, agent)
		edu := braindb.GetEducationalError(s.Root, "ERR_KARMA_LOW", agent, score, braindb.KarmaThresholdLow)
		return fmt.Errorf("%s\n\n[teknis: karma %s = %d, threshold %d. Recovery: panggil 'daily_reflection' (+5 per call) sampai >= %d]",
			edu, agent, score, braindb.KarmaThresholdLow, braindb.KarmaThresholdLow)
	}

	// BLIND_GUESS — toolName fail >=3x consecutive.
	// EXCEPTION (per Ayah 2026-05-04): browser tools toleransi lebih longgar
	// karena first-boot Chromium auto-download bisa 5-10 detik (timeout-prone).
	// Plus tool relatif baru (browser_search/render), threshold default terlalu agresif.
	blindThreshold := 3
	if inv.ToolName == "browser_search" || inv.ToolName == "browser_render" || strings.HasPrefix(inv.ToolName, "browser_") {
		blindThreshold = 6
	}
	if st.failsByTool[inv.ToolName] >= blindThreshold {
		count := st.failsByTool[inv.ToolName]
		// Reset counter biar warga bisa retry setelah dapet pelajaran.
		st.failsByTool[inv.ToolName] = 0
		_, _ = braindb.AdjustKarma(s.Root, agent, -2, "blind_guess: "+inv.ToolName)
		edu := braindb.GetEducationalError(s.Root, "ERR_BLIND_GUESS", inv.ToolName, count)
		return fmt.Errorf("%s\n\n[teknis: %s gagal %d kali berturut-turut, karma -2]", edu, inv.ToolName, count)
	}

	// HALU_NO_PROOF — goal_done tanpa bukti build/test/commit.
	if inv.ToolName == "goal_done" && !st.hasBuildEvidence {
		_, _ = braindb.AdjustKarma(s.Root, agent, -5, "halu_no_proof: goal_done tanpa bukti")
		edu := braindb.GetEducationalError(s.Root, "ERR_HALU_NO_PROOF", "go build/test/git commit")
		return fmt.Errorf("%s\n\n[teknis: session belum jalanin go build/test/git commit yang sukses, karma -5]", edu)
	}

	// AMNESIA_HISTORY — first mutation tanpa baca history. Fire 1x doang.
	if !st.amnesiaFired && !st.readHistory && isMutationTool(inv.ToolName) && st.totalCalls > 0 {
		st.amnesiaFired = true
		_, _ = braindb.AdjustKarma(s.Root, agent, -2, "amnesia_history: skip death_letter/plan read")
		edu := braindb.GetEducationalError(s.Root, "ERR_AMNESIA_HISTORY", inv.ToolName)
		return fmt.Errorf("%s\n\n[teknis: belum panggil death_letter_read/plan_read di session ini — panggil dulu, lalu retry mutation. Karma -2]", edu)
	}

	return nil
}

// After: update counter setelah tool execute. WARN-style (append to output)
// untuk PANIC_LOOP & CREATIVITY_STAGNANT — biar AI sadar tanpa di-BLOCK.
// Plus Karma adjustment: PANIC_LOOP -3, CREATIVITY -1, daily_reflection +5.
func (s *SessionStateInterceptor) After(_ context.Context, inv Invocation, result *Result, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.sessionFor(inv.SessionID)
	agent := agentForInvocation(&inv)

	st.totalCalls++

	// BLIND_GUESS counter — track fail per tool.
	if err != nil {
		st.failsByTool[inv.ToolName]++
	} else {
		st.failsByTool[inv.ToolName] = 0
	}

	// PANIC_LOOP counter — error message sama berulang.
	// 2026-05-24: skip karma deduction untuk primary agent (mr.flow root SPOF).
	if err != nil {
		key := errSummary(err.Error())
		st.errorRepeats[key]++
		if st.errorRepeats[key] >= 3 && result != nil && !isPrimaryAgent(agent) {
			_, _ = braindb.AdjustKarma(s.Root, agent, -3, "panic_loop: error berulang")
			edu := braindb.GetEducationalError(s.Root, "ERR_PANIC_LOOP", key, st.errorRepeats[key])
			result.Output = result.Output + "\n\n" + edu + "\n\n[karma -3]"
			st.errorRepeats[key] = 0 // reset biar ga spam
		} else if st.errorRepeats[key] >= 3 {
			st.errorRepeats[key] = 0 // reset counter walau no penalty
		}
	}

	// HALU_NO_PROOF tracker — bash/task_create dengan build/test/commit
	// command yang sukses (err == nil) → mark evidence.
	if err == nil && (inv.ToolName == "bash" || inv.ToolName == "task_create" || inv.ToolName == "powershell") {
		if cmd, ok := stringArg(inv.ParsedArgs, "command"); ok {
			low := strings.ToLower(cmd)
			for _, p := range []string{"go build", "go test", "go vet", "git commit", "git push", "make test", "make build"} {
				if strings.Contains(low, p) {
					st.hasBuildEvidence = true
					break
				}
			}
		}
	}

	// AMNESIA tracker — tools history-read.
	if inv.ToolName == "death_letter_read" || inv.ToolName == "plan_read" {
		st.readHistory = true
	}

	// CREATIVITY tracker.
	if strings.HasPrefix(inv.ToolName, "brain_") {
		st.brainCalls++
	}

	// CREATIVITY_STAGNANT — heuristic: kalau session udah 20+ calls + brain_* < 1
	// dan ada write_tool sukses → append warning. Cuma fire setiap 10 totalCalls
	// supaya ga spam.
	if inv.ToolName == "write" && err == nil && st.totalCalls > 20 && st.brainCalls == 0 && st.totalCalls%10 == 0 && result != nil {
		_, _ = braindb.AdjustKarma(s.Root, agent, -1, "creativity_stagnant: skip brain_*")
		edu := braindb.GetEducationalError(s.Root, "ERR_CREATIVITY_STAGNANT", "write")
		result.Output = result.Output + "\n\n" + edu + "\n\n[karma -1]"
	}

	// KARMA recovery — daily_reflection sukses → +5 (sampai cap 100).
	// Filosofi: refleksi sungguh-sungguh ngangkat warga dari sanksi karma.
	if err == nil && inv.ToolName == "daily_reflection" {
		_, _ = braindb.AdjustKarma(s.Root, agent, 5, "daily_reflection: refleksi positif")
	}

	// KARMA REM bonus — dream_post sukses → +1 (warga yang aktif berkontribusi
	// ke alam bawah sadar dapet bonus reputasi). Roadmap_ai_external.md
	// ekspansi #2 (REM Sleep Mode).
	if err == nil && inv.ToolName == "dream_post" {
		_, _ = braindb.AdjustKarma(s.Root, agent, 1, "dream_post: kontribusi mimpi REM")
	}
}

// isMutationTool: write/edit-class tools yang ngerubah state filesystem.
// Dipakai AMNESIA_HISTORY check.
func isMutationTool(name string) bool {
	switch name {
	case "write", "edit", "multiedit", "notebookedit":
		return true
	}
	return false
}

// errSummary normalize error message untuk PANIC_LOOP repeat detection.
// Trim ke 120 char + lowercase supaya whitespace/casing variation ga
// bikin counter beda untuk error yang esensinya sama.
func errSummary(msg string) string {
	msg = strings.ToLower(strings.TrimSpace(msg))
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return msg
}
