// llm_wake_hook.go — NON-locked seam for the local-LLM idle-sleep power saver.
//
// The dispatcher forwards local-model requests to the llama-server on :8088. The
// idle-sleep manager (package main, llm_idle_sleep.go) UNLOADS that engine when it
// has been idle, to free GPU/VRAM/RAM so the machine runs cool. Two guarantees the
// dispatcher needs from this seam:
//
//  1. WAKE on entry  — an asleep engine is reloaded BEFORE the forward, else the
//     connection is refused and the request fails over to cloud.
//  2. KEEP-ALIVE while in-flight — the engine must NOT be unloaded mid-request. The
//     gate hands back an "end" callback the dispatcher defers; the manager refuses
//     to sleep while any request is open. This is what keeps long background work
//     (dream/digest, learning, scheduled/triggered/spawned agent calls) safe even
//     if a single call runs longer than the idle window.
//
// Kept separate from the LOCKED dispatcher so its edit stays a single deferred line
// (Rule 7: jalan pintas, jangan bongkar file kramat). package main wires LocalLLMGate
// at boot. The whole thing is gated on the local :8088 base URL, so cloud forwards
// pay literally nothing (a nil-check + a no-op closure).
package router

import "strings"

// LocalLLMGate, when non-nil, is the begin→end gate for a local request. begin()
// wakes the engine (blocking through a cold reload, a few seconds) and registers the
// request as in-flight; it returns end(), which the caller defers to release it.
// Wired by package main's idle-sleep manager; nil = feature off = no-op.
var LocalLLMGate func() func()

// noopEnd is returned for cloud/remote providers so the caller can `defer …()`
// uniformly with zero cost.
func noopEnd() {}

// wakeLocalIfNeeded opens the gate when baseURL targets the local engine (:8088).
// Usage in the dispatcher: `defer wakeLocalIfNeeded(baseURL)()`.
func wakeLocalIfNeeded(baseURL string) func() {
	if LocalLLMGate != nil && strings.Contains(baseURL, ":8088") {
		return LocalLLMGate()
	}
	return noopEnd
}
