// ⚠️ NEW AGENT? READ doc/handbook/menu-ai-agent.md FIRST — enforced rules: secrets→Settings·API Keys, prompt→GUI (kv.prompt), two-tier brain (router+per-agent), bus over fs:shared, extend the frozen kernel via hooks (never unlock). Breaking one is a bug, not a shortcut.
// === LOCKED FILE ===
// Status: STABLE — `thinking` group sequential orchestrator. ITEM 1 + 6-7-8 + 10-11/13
// seed done + tested 2026-06-08. Pipeline: questioner → how → CASTER (picks ≤3 bench
// lenses) → chosen lenses (ONE AT A TIME, synchronous askMember = done-detector) →
// CONNECTOR-synth → recordWiring (lens co-activation edge, success-gated, kv). Bench/
// caster/lenses default in loadRoster, overridable via loket kv.
// Do not edit without owner approval. Rebuild: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
//
// Package main is the Flowork "thinking" group — a SEQUENTIAL colony.
//
// Unlike the generic group template (which broadcasts one task to all members in
// parallel), this orchestrator runs a pipeline that mirrors the owner's way of
// thinking, in order:
//
//  1. QUESTIONER frames the subject into the questions that must be answered (5W+1H).
//  2. Each LENS answers the subject AND those questions through its own grounded
//     way of thinking (each lens retrieves its principles → no hallucination).
//  3. The SYNTHESIZER fuses the lenses into one decision.
//
// It owns NO domain logic and never touches a member's folder — it reaches members
// only through the kernel bus (bus.request). The roster (which agent plays which
// role) is read from a transparent workspace file (roster.json), NOT hardcoded, so
// the same wasm can drive a different thinking colony by editing one file.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

// kvGet reads one key from this group's OWN loket store (live), so roster edits
// from the Group Colony menu apply WITHOUT a restart. "" if absent.
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

func kvSet(k, v string) { _, _ = loketCall("store.kv.set", map[string]any{"k": k, "v": v}) }

// recordWiring is the self-wiring SEED (ROADMAP_THINKING.md item 10-11/13, start-small):
// "firing creates wiring." On a SUCCESSFUL session (we got a real final answer), strengthen
// the co-activation edge between every pair of lenses that fired together. Grounded by
// success (only on a real answer), stored in kv (no kernel change, no lens change). The
// caster/connector can later read these edges to pick + bridge faster. Pattern-level brain
// edges + spreading-activation retrieval remain the deeper Horizon.
func recordWiring(lenses []string) {
	for i := 0; i < len(lenses); i++ {
		for j := i + 1; j < len(lenses); j++ {
			a, b := lenses[i], lenses[j]
			if a > b {
				a, b = b, a
			}
			key := "wire:" + a + "|" + b
			n, _ := strconv.Atoi(strings.TrimSpace(kvGet(key)))
			kvSet(key, strconv.Itoa(n+1))
		}
	}
}

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

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
		"headers":     map[string]string{"Content-Type": "application/json"},
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

// askMember sends one subject to a member ant and returns its human reply. The
// bus.request result is {"reply": <member emit>}, and the member emit is
// {"reply":"…"} — so we unwrap twice. "" on any failure (caller degrades).
func askMember(to, subject string) string {
	r, err := loketCall("bus.request", map[string]any{
		"to": to, "type": "task", "payload": map[string]any{"text": subject},
	})
	if err != nil {
		return ""
	}
	var outer struct {
		Reply json.RawMessage `json:"reply"`
	}
	if json.Unmarshal(r, &outer) != nil || len(outer.Reply) == 0 {
		return ""
	}
	var inner struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal(outer.Reply, &inner) == nil && inner.Reply != "" {
		return inner.Reply
	}
	return string(outer.Reply)
}

// roster is who plays which role — read from workspace/roster.json (transparent,
// editable, no hardcoding). Defaults keep a fresh copy useful out of the box.
// benchLens is one lens available on the bench: its id + a short note on what it is
// good for, so the caster can pick the relevant ones for a given subject.
type benchLens struct {
	ID   string
	Desc string
}

type roster struct {
	Questioner  string
	How         string
	Caster      string      // picks which bench lenses run for THIS subject (item 7)
	Bench       []benchLens // the full set of available lenses (item 6)
	Lenses      []string    // fallback fixed lenses if no caster/bench
	Synthesizer string
}

// loadRoster reads who plays which role from this group's OWN loket store — the
// SAME store the Group Colony menu writes. Convention: "members" = the LENSES
// (the list shown + editable in the menu); "questioner" and "synthesizer" are
// their own keys. Defaults keep it working out of the box before any menu edit.
func loadRoster() roster {
	rs := roster{
		Questioner:  "thinking-questions",
		How:         "thinking-how",
		Caster:      "thinking-caster",
		Bench: []benchLens{
			{"thinking-strategy", "strategy: positioning & tactics vs rivals, win at least cost"},
			{"thinking-improvement", "continuous improvement: get better via small consistent steps"},
			{"thinking-influence", "influence/persuasion: how to convince & move people (honestly)"},
			{"thinking-inversion", "inversion: what would make this fail, and how to avoid it"},
			{"thinking-firstprinciples", "first principles: strip to fundamentals, rebuild from there"},
		},
		Lenses: []string{"thinking-strategy", "thinking-improvement", "thinking-influence", "thinking-inversion", "thinking-firstprinciples"},
		Synthesizer: "thinking-synthesis",
	}
	if q := kvGet("questioner"); q != "" {
		rs.Questioner = q
	}
	if h := kvGet("how_agent"); h != "" {
		rs.How = h
	}
	if c := kvGet("caster"); c != "" {
		rs.Caster = c
	}
	if b := kvGet("bench"); b != "" {
		bench := []benchLens{}
		for _, part := range strings.Split(b, ";") {
			if part = strings.TrimSpace(part); part == "" {
				continue
			}
			id, desc, _ := strings.Cut(part, ":")
			if id = strings.TrimSpace(id); id != "" {
				bench = append(bench, benchLens{ID: id, Desc: strings.TrimSpace(desc)})
			}
		}
		if len(bench) > 0 {
			rs.Bench = bench
		}
	}
	if s := kvGet("synthesizer"); s != "" {
		rs.Synthesizer = s
	}
	if m := kvGet("members"); m != "" {
		lenses := []string{}
		for _, x := range strings.Split(m, ",") {
			if x = strings.TrimSpace(x); x != "" {
				lenses = append(lenses, x)
			}
		}
		if len(lenses) > 0 {
			rs.Lenses = lenses
		}
	}
	return rs
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
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) > 0 {
			args = string(msg.Payload)
		}
		runThink(args)
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

// runThink is the sequential pipeline: questions → lenses (answering the subject +
// the questions) → synthesis. Each stage degrades gracefully so one slow/failed
// member never takes the whole colony down.
func runThink(argsJSON string) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	subject := strings.TrimSpace(in.Text)
	if subject == "" {
		emit(map[string]any{"error": "empty subject"})
		return
	}
	rs := loadRoster()

	// Stage 1 — frame the subject into the key questions.
	questions := ""
	if rs.Questioner != "" {
		questions = askMember(rs.Questioner, "Situation:\n"+subject)
	}

	// Stage 1b — MANUFACTURE candidate paths ("bagaimana caranya") from subject + questions.
	// This is the generator organ: it turns the framed problem into concrete routes, so the
	// lenses below have real options to EVALUATE (not just the bare subject).
	paths := ""
	if rs.How != "" {
		howTask := "Situation/goal:\n" + subject
		if questions != "" {
			howTask += "\n\nKey questions:\n" + questions
		}
		howTask += "\n\nManufacture 3 to 5 concrete, DIFFERENT paths to reach this."
		paths = askMember(rs.How, howTask)
	}

	// Stage 1c — CASTER picks which bench lenses are relevant for THIS subject (item 7):
	// run 2-3 relevant lenses instead of the whole bench, so the bench can grow without
	// every question paying for every lens. Falls back to the fixed lenses if no caster.
	lenses := rs.Lenses
	cast := []string{}
	if rs.Caster != "" && len(rs.Bench) > 0 {
		var bl strings.Builder
		ids := map[string]bool{}
		for _, b := range rs.Bench {
			bl.WriteString(b.ID + ": " + b.Desc + "\n")
			ids[b.ID] = true
		}
		pick := askMember(rs.Caster, "Situation:\n"+subject+"\n\nAvailable lenses:\n"+bl.String()+"\nPick the 2-3 most relevant ids.")
		for _, raw := range strings.FieldsFunc(pick, func(r rune) bool {
			return r == ',' || r == '\n' || r == ' ' || r == ';' || r == '"' || r == '`'
		}) {
			if id := strings.TrimSpace(raw); ids[id] {
				cast = append(cast, id)
			}
		}
		if len(cast) > 0 {
			lenses = cast
			if len(lenses) > 2 { // cap at 2: keeps the 7-stage pipeline well inside the
				lenses = lenses[:2] // call deadline (~65s) while still multi-lens + bridge
			}
		}
	}

	// Stage 2 — every lens EVALUATES the subject, the questions, AND the candidate paths.
	lensTask := "Subject:\n" + subject
	if questions != "" {
		lensTask += "\n\nKey questions to address:\n" + questions
	}
	if paths != "" {
		lensTask += "\n\nProposed candidate paths:\n" + paths
	}
	lensTask += "\n\nAnalyze this subject through your lens, answer the key questions, and evaluate the candidate paths above from your lens point of view."

	sections := []string{}
	if questions != "" {
		sections = append(sections, "### Key questions\n"+questions)
	}
	if paths != "" {
		sections = append(sections, "### Candidate paths\n"+paths)
	}
	// Lenses run ONE AT A TIME (owner directive): askMember is synchronous — it returns
	// only AFTER the member finished, so the call itself is the "done detector"; the
	// next lens starts only once the previous one is complete. No concurrent members.
	lensOut := map[string]string{}
	for _, lens := range lenses {
		ans := askMember(lens, lensTask) // blocks until this lens is DONE, then continue
		if ans == "" {
			ans = "(no answer)"
		}
		lensOut[lens] = ans
		sections = append(sections, "### "+lens+"\n"+ans)
	}
	combined := strings.Join(sections, "\n\n")

	// Stage 3 — synthesize into one decision. No synthesizer → return the sections.
	if rs.Synthesizer == "" {
		emit(map[string]any{"group": selfID(), "reply": combined, "questions": questions, "lenses": lensOut})
		return
	}
	synthInput := "Subject:\n" + subject + "\n\nFindings from each lens:\n\n" + combined +
		"\n\nFuse these into ONE coherent decision: brief reasoning + concrete next steps."
	final := askMember(rs.Synthesizer, synthInput)
	if final == "" {
		// Synthesizer down → degrade to the gathered sections.
		emit(map[string]any{"group": selfID(), "reply": combined, "questions": questions, "lenses": lensOut, "synth_error": "synthesizer no reply"})
		return
	}
	// Success (real final answer) → wire the lenses that fired together (item 10-11/13).
	if len(lenses) >= 2 {
		recordWiring(lenses)
	}
	emit(map[string]any{"group": selfID(), "synthesizer": rs.Synthesizer, "reply": final, "questions": questions, "cast": lenses, "lenses": lensOut})
}
