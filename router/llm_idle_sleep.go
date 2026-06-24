// llm_idle_sleep.go — FROZEN power saver: unload the local LLM when idle, reload on demand.
//
// ROOT CAUSE of "PC panas": the local model (flowork-brain.gguf, ~14GB) stays resident on the
// GPU/RAM via llama-server (:8088) 24/7 — even with nobody chatting — so the card never idles.
// This manager makes the engine EPHEMERAL (mirrors doctrine AOLA-008 Resource Ephemerality):
//
//   - watchdog : every 30s, if the engine has been idle longer than the timeout (default 120s)
//     AND no request is in-flight, Stop() it → frees GPU/VRAM/RAM → the card idles cool.
//   - gate     : the dispatcher's router.LocalLLMGate hook opens a gate around every local
//     request → an asleep engine is reloaded and the request is counted in-flight.
//
// SAFE WITH THE WHOLE ARCHITECTURE (schedule / trigger / spawn / dream / learning): every LLM
// consumer reaches the model through the ONE gateway router :2402 → DispatchChatCompletion(Stream)
// (agents use routerclient → :2402; there is NO direct :8088 caller). The wake/keep-alive gate
// sits in that single path, so background jobs transparently wake the engine and are never killed
// mid-call (in-flight guard). Stop() = Kill+Wait (reap) on an exec-wrapped PID = no zombie/orphan.
// Full design + invariants: lock/AUTOSLEEP.MD.
//
// ❄️ FROZEN brain-pathway — JANGAN edit langsung (chattr +i + KERNEL_FREEZE.md). EXTEND lewat SWITCH
// di bawah, dari file BARU non-frozen (mis. llm_idle_sleep_ext.go): RegisterLLMLifecycleObserver()
// (react ke wake/sleep), SetLLMIdleConfig() (toggle/timeout runtime), LLMIdleStatus() (GUI). Seam
// dispatcher = router.LocalLLMGate. Nambah fitur = file baru manggil switch ini → NOL unfreeze.
//
// Config awal (env, di router/flowork.local.env — per-mesin, gitignored):
//
//	FLOWORK_LLM_IDLE_SLEEP=0        disable (default ON); FLOWORK_LLM_IDLE_SLEEP_SEC=120 (min 10).
//
// Nano-modular: builds ONLY on the exported localai.Runtime API (Start/Stop/Status) + the existing
// localAIRuntimeRef — touches no locked engine code. Wired with one line in main.go.
package main

import (
	"log"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowork-os/flowork_Router/internal/localai"
	"github.com/flowork-os/flowork_Router/internal/router"
)

var (
	// config — atomic so a runtime SWITCH (GUI toggle / policy feature) can retune without a restart.
	llmIdleEnabled     atomic.Bool
	llmIdleTimeoutNano atomic.Int64 // idle window before unload (default 120s)

	llmLastUseNano atomic.Int64 // wall-clock nanos of the last local request (start or end)
	llmInFlight    atomic.Int64 // open local requests — the engine is NEVER unloaded while > 0
	llmAwake       atomic.Bool  // cached engine-up flag (reconciled by the watchdog)
	llmWakeMu      sync.Mutex   // serializes Start/Stop transitions (one reload at a time)

	llmObsMu    sync.Mutex
	llmWakeObs  []func() // fired AFTER a wake  (registered via the SWITCH below)
	llmSleepObs []func() // fired AFTER a sleep
)

// ───────────────────────── SWITCHES (extend WITHOUT unfreezing this file) ─────────────────────────

// RegisterLLMLifecycleObserver registers callbacks fired right AFTER the local engine wakes /
// sleeps. A future feature (GUI status dot, metrics, also-sleep the embedding engine, push a
// notification) adds a NEW non-frozen file calling this from init() — zero edits here. Each
// observer runs under its own recover, so a buggy one can never take down the watchdog. nil-safe.
func RegisterLLMLifecycleObserver(onWake, onSleep func()) {
	llmObsMu.Lock()
	defer llmObsMu.Unlock()
	if onWake != nil {
		llmWakeObs = append(llmWakeObs, onWake)
	}
	if onSleep != nil {
		llmSleepObs = append(llmSleepObs, onSleep)
	}
}

// SetLLMIdleConfig retunes the feature at runtime (e.g. wired to a GUI toggle). enabled=false keeps
// the engine loaded (never auto-unloads); timeoutSec<10 is ignored (current value kept).
func SetLLMIdleConfig(enabled bool, timeoutSec int) {
	llmIdleEnabled.Store(enabled)
	if timeoutSec >= 10 {
		llmIdleTimeoutNano.Store(int64(timeoutSec) * int64(time.Second))
	}
	log.Printf("LLM idle-sleep: config set — enabled=%v timeout=%ds", enabled, timeoutSec)
}

// LLMIdleStatus reports current state (for a GUI/diagnostics endpoint or another feature).
func LLMIdleStatus() (enabled bool, timeoutSec int, awake bool, inFlight int64) {
	return llmIdleEnabled.Load(),
		int(time.Duration(llmIdleTimeoutNano.Load()) / time.Second),
		llmAwake.Load(),
		llmInFlight.Load()
}

func llmFire(obs *[]func()) {
	llmObsMu.Lock()
	list := append([]func(){}, *obs...)
	llmObsMu.Unlock()
	for _, f := range list {
		func() {
			defer func() { _ = recover() }()
			f()
		}()
	}
}

// ───────────────────────────────────────── core ──────────────────────────────────────────────────

func llmIdleTimeout() time.Duration { return time.Duration(llmIdleTimeoutNano.Load()) }
func llmLastUse() time.Time         { return time.Unix(0, llmLastUseNano.Load()) }

// llmIdleSleepInit reads config, wires the dispatcher gate, and launches the idle watchdog.
// Called once from main() at boot. Safe no-op (gate unwired) when disabled.
func llmIdleSleepInit() {
	llmIdleTimeoutNano.Store(int64(120 * time.Second))
	if os.Getenv("FLOWORK_LLM_IDLE_SLEEP") == "0" {
		log.Printf("LLM idle-sleep: disabled (FLOWORK_LLM_IDLE_SLEEP=0)")
		return
	}
	if s := os.Getenv("FLOWORK_LLM_IDLE_SLEEP_SEC"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 10 {
			llmIdleTimeoutNano.Store(int64(n) * int64(time.Second))
		}
	}
	llmIdleEnabled.Store(true)
	llmLastUseNano.Store(time.Now().UnixNano()) // count idle from boot
	if rt := llmCurrentRuntime(); rt != nil && rt.Status().Running {
		llmAwake.Store(true)
	}
	router.LocalLLMGate = llmGateBegin // dispatcher opens this around every :8088 request
	go llmIdleWatchdog()
	log.Printf("LLM idle-sleep: ON — unload flowork-brain after %s idle, reload on demand", llmIdleTimeout())
}

// llmGateBegin — wake the engine, register the request in-flight, return the release callback the
// dispatcher defers. Fast path (already awake) is a couple of atomics, so hot traffic is ~free.
func llmGateBegin() func() {
	llmInFlight.Add(1)
	llmEnsureLocalAwake()
	return func() {
		llmLastUseNano.Store(time.Now().UnixNano()) // idle counts from the END of the request
		llmInFlight.Add(-1)
	}
}

// llmCurrentRuntime returns the shared runtime, lazily creating it if a local binary resolves
// (so wake works even when boot autostart was off). nil = no local engine.
func llmCurrentRuntime() *localai.Runtime {
	localAIRuntimeMu.Lock()
	defer localAIRuntimeMu.Unlock()
	if localAIRuntimeRef == nil {
		bin := localai.ResolveLlamaBin()
		if bin == "" {
			return nil
		}
		localAIRuntimeRef = localai.NewRuntime(bin, 0)
	}
	return localAIRuntimeRef
}

// llmEnsureLocalAwake reloads the engine if asleep. Idempotent; serialized so only one cold reload
// happens under a thundering herd. Fires wake observers on a real (re)load.
func llmEnsureLocalAwake() {
	llmLastUseNano.Store(time.Now().UnixNano())
	if llmAwake.Load() {
		return
	}
	llmWakeMu.Lock()
	defer llmWakeMu.Unlock()
	if llmAwake.Load() {
		return
	}
	rt := llmCurrentRuntime()
	if rt == nil {
		return
	}
	if rt.Status().Healthy {
		llmAwake.Store(true)
		return
	}
	log.Printf("LLM idle-sleep: waking flowork-brain (cold reload)…")
	if err := rt.Start(localai.FloworkBrainModel, ""); err != nil {
		log.Printf("LLM idle-sleep: wake FAILED (%v) — request falls back to cloud/priority", err)
		return
	}
	llmAwake.Store(true)
	log.Printf("LLM idle-sleep: flowork-brain UP on :8088 (woke on demand)")
	llmFire(&llmWakeObs)
}

// llmIdleWatchdog stops the engine after the idle window — but only when enabled and nothing is in
// flight. Reconciles the awake cache with real process state each tick (reaps a crashed engine
// within 30s). Fires sleep observers on a real unload.
func llmIdleWatchdog() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		rt := llmCurrentRuntime()
		if rt == nil {
			continue
		}
		if !rt.Status().Running {
			llmAwake.Store(false)
			continue
		}
		llmAwake.Store(true)
		if !llmIdleEnabled.Load() {
			continue // feature toggled off at runtime → keep engine loaded
		}
		if llmInFlight.Load() > 0 {
			continue // a request (incl. a long background task) is open — keep it loaded
		}
		if time.Since(llmLastUse()) < llmIdleTimeout() {
			continue
		}
		llmWakeMu.Lock()
		// Re-check under the lock with FRESH state: never kill an engine that just got used.
		stopped := false
		if rt.Status().Running && llmInFlight.Load() == 0 && llmIdleEnabled.Load() && time.Since(llmLastUse()) >= llmIdleTimeout() {
			_ = rt.Stop() // Kill + Wait → reaps the exec-wrapped PID, no zombie
			llmAwake.Store(false)
			stopped = true
			log.Printf("LLM idle-sleep: unloaded flowork-brain after %s idle — GPU/RAM freed, PC cools down", llmIdleTimeout())
		}
		llmWakeMu.Unlock()
		if stopped {
			llmFire(&llmSleepObs)
		}
	}
}
