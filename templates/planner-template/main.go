// === LOCKED FILE ===
// Status: STABLE — `thinking-planner` (ROADMAP_THINKING.md item 4-5). Tested 2026-06-08:
// plan→6 steps stored in kv · status reads ground-truth · done is mechanical · progress
// persists from the STORE (not model memory). Do not edit without owner approval.
// Rebuild: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
//
// Package main is the Flowork "planner" — the PLAN half of disciplined execution.
//
// Split-by-design (ROADMAP_THINKING.md item 5): a plan has two parts that must NOT
// be the same thing. The PLANNER reasons (an LLM job: frame with 5W+1H, generate
// steps with "how") — it is allowed to be creative. The LIST is a deterministic
// store (loket kv) — it is NOT an LLM, so it can never "remember" a step as done
// when it isn't. The planner only WRITES to the list; step status is mechanical
// truth read straight from the store. That kills hallucinated progress.
//
// Functions:
//   plan   {goal|text}  → LLM drafts steps, writes each to kv (status=pending), returns them
//   status {}           → reads the list from kv (ground truth) and returns it
//   done   {n}          → marks step n done in kv (mechanical, no LLM)
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unsafe"
)

func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

var outBuf [262144]byte

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}
func emit(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }
func selfID() string { return os.Getenv("FLOWORK_AGENT_ID") }

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})
	reqJSON, _ := json.Marshal(map[string]any{
		"method": "POST", "url": loketURL, "timeout_ms": 120000, "max_resp_bytes": 4 << 20,
		"headers": map[string]string{"Content-Type": "application/json"},
		"body_base64": base64.StdEncoding.EncodeToString(body),
	})
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return nil, fmt.Errorf("host_net_fetch: 0 bytes")
	}
	var host struct {
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:n], &host); err != nil {
		return nil, err
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
		return nil, err
	}
	if !res.OK {
		return nil, fmt.Errorf("loket refused: %s", res.Error)
	}
	return res.Result, nil
}

func kvSet(k, v string) { _, _ = loketCall("store.kv.set", map[string]any{"k": k, "v": v}) }
func kvGet(k string) string {
	r, err := loketCall("store.kv.get", map[string]any{"k": k})
	if err != nil {
		return ""
	}
	var s struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(r, &s) != nil {
		return ""
	}
	return s.Value
}

type step struct {
	N      int    `json:"n"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

const defaultPlannerPrompt = "You are a PLANNER. Given a goal, produce a numbered plan of 3 to 6 concrete, ACTIONABLE, testable steps to reach it. Frame with 5W+1H, generate with 'how'. Output ONLY the numbered steps, one line each, no preamble."

func llmModel() string {
	if m := strings.TrimSpace(os.Getenv("FLOWORK_LLM_MODEL")); m != "" {
		return m
	}
	return "claude-haiku-4-5"
}

var numbered = regexp.MustCompile(`^\s*(\d+)[.)]\s+(.*)$`)

// plan: the LLM drafts steps; we WRITE each into the deterministic kv list.
func plan(goal string) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		emit(map[string]any{"error": "goal kosong"})
		return
	}
	persona := readWS("prompt.md")
	if persona == "" {
		persona = defaultPlannerPrompt
	}
	doktrin := readWS("doktrin.md")
	msgs := []map[string]string{}
	if doktrin != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": doktrin})
	}
	msgs = append(msgs, map[string]string{"role": "system", "content": persona})
	msgs = append(msgs, map[string]string{"role": "user", "content": "Goal:\n" + goal})

	draft := ""
	if r, err := loketCall("llm.complete", map[string]any{"model": llmModel(), "messages": msgs}); err == nil {
		var s struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(r, &s)
		draft = s.Content
	} else {
		emit(map[string]any{"error": "llm offline: " + err.Error()})
		return
	}

	// Parse the draft into discrete steps, then WRITE them to the deterministic store.
	steps := []step{}
	for _, line := range strings.Split(draft, "\n") {
		if m := numbered.FindStringSubmatch(line); m != nil {
			t := strings.TrimSpace(m[2])
			if t != "" {
				steps = append(steps, step{N: len(steps) + 1, Text: t, Status: "pending"})
			}
		}
	}
	if len(steps) == 0 { // model didn't number them → keep the raw draft as one step
		steps = append(steps, step{N: 1, Text: strings.TrimSpace(draft), Status: "pending"})
	}
	kvSet("plan_goal", goal)
	kvSet("plan_count", strconv.Itoa(len(steps)))
	for _, st := range steps {
		b, _ := json.Marshal(st)
		kvSet("step:"+strconv.Itoa(st.N), string(b))
	}
	emit(map[string]any{"reply": draft, "goal": goal, "steps": steps, "stored": len(steps), "agent": selfID()})
}

// readSteps rebuilds the list from the STORE (ground truth, never from memory).
func readSteps() []step {
	n, _ := strconv.Atoi(strings.TrimSpace(kvGet("plan_count")))
	out := []step{}
	for i := 1; i <= n; i++ {
		raw := kvGet("step:" + strconv.Itoa(i))
		if raw == "" {
			continue
		}
		var st step
		if json.Unmarshal([]byte(raw), &st) == nil {
			out = append(out, st)
		}
	}
	return out
}

func status() {
	steps := readSteps()
	done := 0
	for _, s := range steps {
		if s.Status == "done" {
			done++
		}
	}
	emit(map[string]any{"goal": kvGet("plan_goal"), "steps": steps, "done": done, "total": len(steps), "agent": selfID()})
}

// done marks step n complete — MECHANICAL, straight to the store. No LLM, so the
// list can never lie about progress.
func done(n int) {
	raw := kvGet("step:" + strconv.Itoa(n))
	if raw == "" {
		emit(map[string]any{"error": "step tidak ada: " + strconv.Itoa(n)})
		return
	}
	var st step
	_ = json.Unmarshal([]byte(raw), &st)
	st.Status = "done"
	b, _ := json.Marshal(st)
	kvSet("step:"+strconv.Itoa(n), string(b))
	status()
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
	// bus invocation arrives at "handle" with a {payload:{…}} envelope.
	if fn == "handle" {
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) > 0 {
			args = string(msg.Payload)
		}
		fn = "plan"
	}
	var in struct {
		Text string `json:"text"`
		Goal string `json:"goal"`
		N    int    `json:"n"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	goal := in.Goal
	if goal == "" {
		goal = in.Text
	}
	switch fn {
	case "plan", "handle_message":
		plan(goal)
	case "status":
		status()
	case "done":
		done(in.N)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}
