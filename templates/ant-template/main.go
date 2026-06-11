// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// Package main is the Flowork "ant" template — a minimal, loket-native module.
//
// It is the golden reference for the "pasukan semut" (ant army): a tiny
// specialist that does ONE job and reaches EVERY capability through the single
// kernel counter (the loket) via call(cap, args). To make another ant, copy
// this folder, change the manifest name + persona, and pick the capabilities it
// consumes. Small prompt, one job — so even a small/local model can run it.
//
// It is loaded by the existing wasm runtime (command pattern: the kernel runs
// the module with os.Args = [name, function, argsJSON] and reads stdout), but it
// does all of its real work by calling the NEW loket at /api/kernel/call. The
// host injects this module's verified id + the loopback secret on every outbound
// request, so the kernel always knows which ant is calling — un-forgeable.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

// readWS reads a file from this agent's OWN folder (mounted at /workspace). The
// persona and the doctrine live there as plain, transparent, editable files that
// travel with the folder — prompt.md (who the ant is) and doktrin.md (the sacred
// rules it must always obey, e.g. anti-halu). Returns "" if the file is absent.
func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

// outBuf receives the host's response. 256KB is plenty for a loket Result.
var outBuf [262144]byte

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func emit(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func selfID() string { return os.Getenv("FLOWORK_AGENT_ID") }

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

// loketCall is the ant's ONLY door to the world: ask the kernel for a capability
// by name. Returns the raw "result" field on success, or an error if the kernel
// refused (capability not granted) or the provider failed.
func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})

	reqJSON, _ := json.Marshal(map[string]any{
		"method":         "POST",
		"url":            loketURL,
		"timeout_ms":     120000,
		"max_resp_bytes": 4 << 20,
		"headers":        map[string]string{"Content-Type": "application/json"},
		"body_base64":    base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return nil, fmt.Errorf("host_net_fetch: 0 bytes")
	}
	var host struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:n], &host); err != nil {
		return nil, fmt.Errorf("host decode: %w", err)
	}
	if host.Error != "" {
		return nil, fmt.Errorf("host: %s", host.Error)
	}
	raw, _ := base64.StdEncoding.DecodeString(host.BodyB64)
	var res struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("loket decode: %w (body=%s)", err, string(raw))
	}
	if !res.OK {
		return nil, fmt.Errorf("loket refused: %s", res.Error)
	}
	return res.Result, nil
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	fn := os.Args[1]
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch fn {
	case "handle_message":
		// Direct/RPC invocation: args is the payload itself, e.g. {"text":"..."}.
		handleMessage(args)
	case "handle":
		// Loket-bus invocation: args is a loket Message — unwrap its payload so a
		// group can route work to this ant the same way a chat does.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) == 0 {
			msg.Payload = json.RawMessage(args)
		}
		handleMessage(string(msg.Payload))
	case "boot":
		emit(map[string]any{"ok": true}) // template has no daemon loop
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}

// agentConfig is this ant's persona + model, injected by the host from the
// agent's own config (FLOWORK_AGENT_CONFIG). This is the whole "copas" recipe:
// the SAME wasm becomes a different ant just by changing its config + manifest —
// no code change. Defaults keep a fresh copy useful out of the box.
type agentConfig struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func loadConfig() agentConfig {
	c := agentConfig{
		Prompt: "You are a concise title writer. Reply with ONE short, catchy title and nothing else.",
		Model:  "claude-haiku-4-5",
	}
	if raw := os.Getenv("FLOWORK_AGENT_CONFIG"); raw != "" {
		var parsed agentConfig
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			if parsed.Prompt != "" {
				c.Prompt = parsed.Prompt
			}
			if parsed.Model != "" {
				c.Model = parsed.Model
			}
		}
	}
	return c
}

// handleMessage is this ant's single job. It proves the loket end to end: it
// remembers the message in its OWN isolated brain, recalls related memories, and
// (if the LLM service is up) writes a short reply — every step through the loket.
func handleMessage(argsJSON string) {
	var in struct {
		Text string `json:"text"`
		User string `json:"user"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	if in.Text == "" {
		emit(map[string]any{"reply": "(empty message)"})
		return
	}

	steps := map[string]any{}

	// 1. Remember the message in our own isolated brain (store.brain.add).
	if _, err := loketCall("store.brain.add", map[string]any{"content": in.Text, "wing": "experience"}); err != nil {
		steps["brain_add"] = "err: " + err.Error()
	} else {
		steps["brain_add"] = "ok"
	}

	// 2. Note the last message (store.kv.set).
	if _, err := loketCall("store.kv.set", map[string]any{"k": "last_message", "v": in.Text}); err != nil {
		steps["kv_set"] = "err: " + err.Error()
	} else {
		steps["kv_set"] = "ok"
	}

	// 3. Recall related memories (store.brain.search).
	hits := 0
	if r, err := loketCall("store.brain.search", map[string]any{"query": in.Text, "k": 3}); err == nil {
		var s struct {
			Count int `json:"count"`
		}
		_ = json.Unmarshal(r, &s)
		hits = s.Count
		steps["brain_search"] = "ok"
	} else {
		steps["brain_search"] = "err: " + err.Error()
	}

	// 4. Ask the LLM SERVICE (a small model). Tolerate it being offline — the
	//    store steps above already prove the loket works without the router.
	reply := ""
	cfg := loadConfig()
	// Persona + doctrine come from transparent files in the agent's OWN folder
	// (prompt.md / doktrin.md). Fall back to config, then to a built-in default,
	// so a fresh copy still works out of the box. The doctrine goes FIRST as a
	// system message — it is the sacred, always-injected anti-halu layer.
	persona := readWS("prompt.md")
	if persona == "" {
		persona = cfg.Prompt
	}
	doktrin := readWS("doktrin.md")

	msgs := []map[string]string{}
	if doktrin != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": doktrin})
	}
	msgs = append(msgs, map[string]string{"role": "system", "content": persona})
	msgs = append(msgs, map[string]string{"role": "user", "content": in.Text})
	llmArgs := map[string]any{"model": cfg.Model, "messages": msgs}
	if r, err := loketCall("llm.complete", llmArgs); err == nil {
		var s struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(r, &s)
		reply = s.Content
		steps["llm"] = "ok"
	} else {
		steps["llm"] = "offline: " + err.Error()
		reply = fmt.Sprintf("[title-writer] remembered your message (%d related memories). LLM offline.", hits)
	}

	emit(map[string]any{"reply": reply, "agent": selfID(), "loket_steps": steps})
}
