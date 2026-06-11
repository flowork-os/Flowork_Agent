// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked at: 2026-06-06
// Reason: Group template wasm — audited. Solid error handling on every loketCall,
//   empty-members guard, graceful synthesizer-down degrade, live roster read. Buffer
//   raised 256KB→512KB (respBufBytes) to match the platform standard, since a group
//   aggregates many member replies in one bus.broadcast. Rebuild: GOOS=wasip1
//   GOARCH=wasm go build -o agent.wasm .
//
// Package main is the Flowork "group" template — a colony of ants.
//
// A group is itself a module. It owns NO domain logic; its single job is to
// route one task to its MEMBER ants (via the loket bus) and gather their
// answers. Members are listed in the group's own config (no hardcoding), so a
// new team = copy this folder + set members + tasks. The group reaches its
// members only through the kernel (bus.broadcast) — it never touches another
// module's folder, so isolation holds.
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

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

// respBufBytes caps one host_net_fetch response copied back into the guest. A group
// AGGREGATES every member's reply inside a single bus.broadcast result, so it's the
// most likely module to overflow a small buffer — use the platform-standard 512KB
// (same as mr-flow). An over-large fan-out then degrades (parse skips), never crashes.
const respBufBytes = 524288

var outBuf [respBufBytes]byte

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

// groupCfg is the group's roster + job, read from its OWN config. Arbitrary keys
// land under "kv" in FLOWORK_AGENT_CONFIG (only prompt/router are promoted to the
// top level). A new team = copy this folder + set these three keys, no code edit:
//   - members:     comma-separated WORKER ant ids (each does one angle of the job)
//   - synthesizer: one ant id that combines the workers' answers (optional)
//   - task:        a short framing prepended to the user's text (optional)
type groupCfg struct {
	Members     []string
	Synthesizer string
	Task        string
}

// kvGet reads one key from this group's OWN loket store (live). Reading live (not
// from the boot-frozen FLOWORK_AGENT_CONFIG) means roster/task edits from the GUI
// apply WITHOUT a restart. Empty string if absent.
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
	return strings.TrimSpace(s.Value)
}

func readCfg() groupCfg {
	out := groupCfg{Synthesizer: kvGet("synthesizer"), Task: kvGet("task")}
	for _, m := range strings.Split(kvGet("members"), ",") {
		if m = strings.TrimSpace(m); m != "" {
			out.Members = append(out.Members, m)
		}
	}
	return out
}

// replyText pulls the human answer out of a member ant's raw emit, which is
// {"reply":"…","agent":"…"}. Falls back to the raw JSON if the shape differs.
func replyText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var r struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal(raw, &r) == nil && r.Reply != "" {
		return r.Reply
	}
	return string(raw)
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch os.Args[1] {
	case "handle_message", "handle":
		// Unwrap a loket Message payload if present (group can be triggered by
		// a channel or another module), else treat args as the payload directly.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) > 0 {
			args = string(msg.Payload)
		}
		runTask(args)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

// runTask is the whole point of a group: fan ONE task out to the worker ants,
// gather their angles, then (if configured) hand all of them to a SYNTHESIZER ant
// that fuses them into one answer. Each ant carries a tiny, single-job prompt, so
// the whole pipeline runs on a small/local model without any over-prompting — the
// "pasukan semut" pattern. The group reaches members ONLY through the kernel bus,
// never their folders, so isolation holds.
func runTask(argsJSON string) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	cfg := readCfg()
	if len(cfg.Members) == 0 {
		emit(map[string]any{"error": "group has no members (set config.members)"})
		return
	}

	// Frame the job: the group's task description (config) + the user's request.
	subject := strings.TrimSpace(in.Text)
	taskText := subject
	if cfg.Task != "" {
		taskText = cfg.Task + "\n\n" + subject
	}

	// 1. Fan out to every worker ant; each returns its own angle.
	r, err := loketCall("bus.broadcast", map[string]any{
		"to":      cfg.Members,
		"type":    "task",
		"payload": map[string]any{"text": taskText},
	})
	if err != nil {
		emit(map[string]any{"error": err.Error(), "members": cfg.Members})
		return
	}
	var bc struct {
		Replies []struct {
			Target string          `json:"target"`
			Reply  json.RawMessage `json:"reply"`
			Error  string          `json:"error"`
		} `json:"replies"`
	}
	_ = json.Unmarshal(r, &bc)

	// 2. Collect each worker's answer as a labelled section.
	var sections []string
	worker := map[string]string{}
	for _, rep := range bc.Replies {
		txt := replyText(rep.Reply)
		if rep.Error != "" {
			txt = "(error: " + rep.Error + ")"
		}
		worker[rep.Target] = txt
		sections = append(sections, "### "+rep.Target+"\n"+txt)
	}
	combined := strings.Join(sections, "\n\n")

	// 3. No synthesizer → return the gathered angles as-is.
	if cfg.Synthesizer == "" {
		emit(map[string]any{"group": selfID(), "members": cfg.Members, "reply": combined, "workers": worker})
		return
	}

	// 4. Hand all worker angles to the synthesizer ant; it fuses one answer.
	synthInput := "Pertanyaan/subjek:\n" + subject + "\n\nHasil analisa tiap anggota tim:\n\n" + combined +
		"\n\nGabungkan jadi SATU kesimpulan utuh yang seimbang."
	sr, serr := loketCall("bus.request", map[string]any{
		"to":      cfg.Synthesizer,
		"type":    "synthesize",
		"payload": map[string]any{"text": synthInput},
	})
	if serr != nil {
		// Synthesizer down → degrade gracefully to the collected angles.
		emit(map[string]any{"group": selfID(), "members": cfg.Members, "reply": combined, "workers": worker, "synth_error": serr.Error()})
		return
	}
	var outer struct {
		Reply json.RawMessage `json:"reply"`
	}
	_ = json.Unmarshal(sr, &outer)
	final := replyText(outer.Reply)
	emit(map[string]any{"group": selfID(), "members": cfg.Members, "synthesizer": cfg.Synthesizer, "reply": final, "workers": worker})
}
