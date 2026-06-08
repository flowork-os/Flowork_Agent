// === LOCKED FILE ===
// Status: STABLE — `thinking` group sequential orchestrator. ITEM 1 (how-stage)
// done + tested end-to-end via chat path 2026-06-08 (questioner→how→lenses→synth,
// ~50s, no timeout). Members run ONE AT A TIME (synchronous askMember = done-detector).
// Unlock only with owner intent for ITEM 6-9 (bench/caster/connector reshape).
// Rebuild: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
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
type roster struct {
	Questioner  string   `json:"questioner"`
	How         string   `json:"how"`
	Lenses      []string `json:"lenses"`
	Synthesizer string   `json:"synthesizer"`
}

// loadRoster reads who plays which role from this group's OWN loket store — the
// SAME store the Group Colony menu writes. Convention: "members" = the LENSES
// (the list shown + editable in the menu); "questioner" and "synthesizer" are
// their own keys. Defaults keep it working out of the box before any menu edit.
func loadRoster() roster {
	rs := roster{
		Questioner:  "thinking-questions",
		How:         "thinking-how",
		Lenses:      []string{"thinking-strategy", "thinking-improvement"},
		Synthesizer: "thinking-synthesis",
	}
	if q := kvGet("questioner"); q != "" {
		rs.Questioner = q
	}
	if h := kvGet("how_agent"); h != "" {
		rs.How = h
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
		questions = askMember(rs.Questioner, "Situasi:\n"+subject)
	}

	// Stage 1b — MANUFACTURE candidate paths ("bagaimana caranya") from subject + questions.
	// This is the generator organ: it turns the framed problem into concrete routes, so the
	// lenses below have real options to EVALUATE (not just the bare subject).
	paths := ""
	if rs.How != "" {
		howTask := "Situasi/goal:\n" + subject
		if questions != "" {
			howTask += "\n\nPertanyaan kunci:\n" + questions
		}
		howTask += "\n\nManufaktur 3-5 jalan konkret yang BERBEDA untuk mencapai ini."
		paths = askMember(rs.How, howTask)
	}

	// Stage 2 — every lens EVALUATES the subject, the questions, AND the candidate paths.
	lensTask := "Subjek:\n" + subject
	if questions != "" {
		lensTask += "\n\nPertanyaan kunci yang perlu dijawab:\n" + questions
	}
	if paths != "" {
		lensTask += "\n\nKandidat jalan yang diusulkan:\n" + paths
	}
	lensTask += "\n\nAnalisa subjek ini lewat lensa kamu, jawab pertanyaan kunci, dan nilai kandidat jalan di atas dari sudut pandang lensamu."

	sections := []string{}
	if questions != "" {
		sections = append(sections, "### Pertanyaan kunci\n"+questions)
	}
	if paths != "" {
		sections = append(sections, "### Kandidat jalan\n"+paths)
	}
	// Lenses run ONE AT A TIME (owner directive): askMember is synchronous — it returns
	// only AFTER the member finished, so the call itself is the "done detector"; the
	// next lens starts only once the previous one is complete. No concurrent members.
	lensOut := map[string]string{}
	for _, lens := range rs.Lenses {
		ans := askMember(lens, lensTask) // blocks until this lens is DONE, then continue
		if ans == "" {
			ans = "(tidak ada jawaban)"
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
	synthInput := "Subjek:\n" + subject + "\n\nHasil tiap sudut pandang:\n\n" + combined +
		"\n\nGabungkan jadi SATU keputusan utuh: alasan ringkas + langkah konkret."
	final := askMember(rs.Synthesizer, synthInput)
	if final == "" {
		// Synthesizer down → degrade to the gathered sections.
		emit(map[string]any{"group": selfID(), "reply": combined, "questions": questions, "lenses": lensOut, "synth_error": "synthesizer no reply"})
		return
	}
	emit(map[string]any{"group": selfID(), "synthesizer": rs.Synthesizer, "reply": final, "questions": questions, "lenses": lensOut})
}
