// llm_idle_sleep_ext.go — NON-frozen GROWTH POINT for the autosleep feature.
//
// The core (llm_idle_sleep.go + internal/router/llm_wake_hook.go) is FROZEN. To add behavior to
// the autosleep WITHOUT unfreezing anything, DON'T edit the frozen files — add a NEW non-frozen
// file (like this one) and register through the exported SWITCHES:
//
//	RegisterLLMLifecycleObserver(onWake, onSleep)  → react when the engine wakes / sleeps
//	SetLLMIdleConfig(enabled, timeoutSec)          → toggle / retune at runtime (e.g. GUI button)
//	LLMIdleStatus()                                → read state for a GUI/diagnostics view
//	router.LocalLLMGate (advanced)                 → wrap the per-request wake gate
//
// TEMPLATE — copy into a feature_xxx.go to extend, no freeze touched:
//
//	func init() {
//	    RegisterLLMLifecycleObserver(
//	        func() { /* on wake: e.g. light a GUI dot, start metrics */ },
//	        func() { /* on sleep: e.g. also unload the embedding engine, notify */ },
//	    )
//	}
//
// Below: a tiny, opt-in example (off by default → zero behavior change) that doubles as a smoke
// test proving the switch is wired. Enable with FLOWORK_LLM_IDLE_DEBUG=1.
package main

import (
	"log"
	"os"
)

func init() {
	if os.Getenv("FLOWORK_LLM_IDLE_DEBUG") != "1" {
		return
	}
	RegisterLLMLifecycleObserver(
		func() { log.Printf("LLM idle-sleep[ext]: observer — engine WOKE") },
		func() { log.Printf("LLM idle-sleep[ext]: observer — engine SLEPT") },
	)
}
