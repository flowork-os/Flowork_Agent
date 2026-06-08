// Package main is Mr.Flow, loket-native — the first mature agent migrated to the
// "papan kosong" (blank board) microkernel. It reaches EVERY capability through
// the single kernel counter call(cap, args): its own brain, the real clock, and
// the LLM service. It owns no privileged code of its own.
//
// Built ALONGSIDE the legacy mr-flow (non-destructive migration): the old agent
// stays live until this one is proven, then we swap. Persona (prompt.md) and the
// sacred doctrine (doktrin.md) live as transparent, editable files in this
// agent's OWN folder and travel with it — nothing the owner cannot read + change.
//
// Phase A scope: an HONEST chat core — recall brain, ground on the real clock,
// obey the doctrine, answer in Mr.Flow's voice. No fake tools and no daemon yet:
// tools (Phase C) and the Telegram channel (Phase D) arrive as separate modules,
// so the agent never claims a capability it does not actually have (anti-halu).
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

// selfID is this agent's id, injected by the host via FLOWORK_AGENT_ID.
func selfID() string {
	if id := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_ID")); id != "" {
		return id
	}
	return "mr-flow-next"
}

// readWS reads a file from this agent's OWN folder (mounted at /workspace). The
// persona (prompt.md) and the sacred doctrine (doktrin.md) live there as plain,
// transparent files that travel with the folder. Returns "" if absent.
func readWS(name string) string {
	b, err := os.ReadFile("/workspace/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

// loketCall is the agent's ONLY door to the world: ask the kernel for a
// capability by name. The host stamps our verified id + the loopback secret on
// the outbound request, so the kernel always knows it is us — un-forgeable.
// Returns the raw "result" on success, or an error if the kernel refused.
func loketCall(capName string, args any) (json.RawMessage, error) {
	return loketCallT(capName, args, 120000)
}

// loketCallT is loketCall with an explicit host-fetch timeout (ms). The LLM call
// uses a SHORTER bound than the response deadline so the orchestrator can fall back
// to a cheaper model instead of hanging when the premium tier is rate-limited.
func loketCallT(capName string, args any, timeoutMs int) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})

	reqJSON, _ := json.Marshal(map[string]any{
		"method":         "POST",
		"url":            loketURL,
		"timeout_ms":     timeoutMs,
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

// defaultModel — a SMALL model by default (the ant ethos: sovereign, runs local).
// Mr.Flow is the commander, but Phase A is plain chat; flip to a larger model in
// config when the orchestration tools land. Overridable via FLOWORK_AGENT_CONFIG.
const defaultModel = "claude-haiku-4-5"

// model resolves which model to use: config override, else the small default.
func model() string {
	raw := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_CONFIG"))
	if raw == "" {
		return defaultModel
	}
	var c struct {
		Router struct {
			Model string `json:"model"`
		} `json:"router"`
		Model string `json:"model"`
	}
	if json.Unmarshal([]byte(raw), &c) == nil {
		if c.Router.Model != "" {
			return c.Router.Model
		}
		if c.Model != "" {
			return c.Model
		}
	}
	return defaultModel
}

func main() {
	if len(os.Args) < 2 {
		emit(map[string]string{"error": "missing function"})
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
		// channel or a group can route work to this agent the same way a chat does.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) == 0 {
			msg.Payload = json.RawMessage(args)
		}
		handleMessage(string(msg.Payload))
	case "boot":
		// No daemon yet — Telegram arrives via a separate channel module (Phase D).
		emit(map[string]any{"ok": true})
	case "on_load":
		// Lifecycle (§8.A): init. Record our load time in our own store.
		if ts := nowUTC(); ts != "" {
			_, _ = loketCall("store.kv.set", map[string]any{"k": "last_load", "v": ts})
		}
		emit(map[string]any{"ok": true})
	case "on_stop":
		// Lifecycle (§8.A): death-letter — leave a wasiat in our OWN brain before we go.
		_, _ = loketCall("store.brain.add", map[string]any{
			"content": "[death-letter] mr-flow-next stopped at " + nowUTC(),
			"wing":    "experience",
			"room":    "lifecycle",
		})
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}

// nowUTC asks the kernel for the real clock (time.now). Grounding the model on
// the true time is part of anti-halu: never let it guess "today".
func nowUTC() string {
	r, err := loketCall("time.now", map[string]any{})
	if err != nil {
		return ""
	}
	var s struct {
		TS string `json:"ts"`
	}
	_ = json.Unmarshal(r, &s)
	return s.TS
}

// recall searches our OWN isolated brain for memories related to the message, so
// the answer is grounded in what we actually remember — not invented. Returns the
// joined memory block and the hit count (the count feeds the debug affordance).
func recall(text string) (string, int) {
	r, err := loketCall("store.brain.search", map[string]any{"query": text, "k": 3})
	if err != nil {
		return "", 0
	}
	var s struct {
		Hits []struct {
			Content string `json:"content"`
		} `json:"hits"`
	}
	if json.Unmarshal(r, &s) != nil || len(s.Hits) == 0 {
		return "", 0
	}
	var b strings.Builder
	n := 0
	for _, h := range s.Hits {
		if c := strings.TrimSpace(h.Content); c != "" {
			b.WriteString("- ")
			b.WriteString(c)
			b.WriteString("\n")
			n++
		}
	}
	return strings.TrimSpace(b.String()), n
}

// searchShared queries the 5M shared corpus (brain.shared.search) — a PRIMARY-tier
// privilege. The kernel REFUSES it for extension agents (the tier gate); this same
// code then simply returns "" and the agent grounds on its local brain only. So one
// wasm stays correct at either tier, with the KERNEL — not the agent — enforcing.
func searchShared(query string) (text string, n int, status string) {
	r, err := loketCall("brain.shared.search", map[string]any{"query": query, "k": 3})
	if err != nil {
		// A refusal here is the tier gate doing its job for an extension agent.
		return "", 0, "refused/err: " + err.Error()
	}
	var s struct {
		Hits []struct {
			Content string `json:"content"`
		} `json:"hits"`
	}
	if json.Unmarshal(r, &s) != nil {
		return "", 0, "decode-err"
	}
	var b strings.Builder
	for _, h := range s.Hits {
		c := strings.TrimSpace(h.Content)
		if c == "" {
			continue
		}
		if len(c) > 600 {
			c = c[:600] + "…"
		}
		b.WriteString("- ")
		b.WriteString(c)
		b.WriteString("\n")
		n++
	}
	return strings.TrimSpace(b.String()), n, "ok"
}

// handleMessage is Mr.Flow's chat core. Every step goes through the loket:
// recall brain → ground on the real clock → obey the doctrine → answer in
// Mr.Flow's voice → remember the exchange.
func handleMessage(argsJSON string) {
	var in struct {
		Text   string `json:"text"`
		User   string `json:"user"`
		ChatID int64  `json:"chat_id"`
		Debug  bool   `json:"debug"` // owner diagnostic: report grounding sources used
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		emit(map[string]any{"reply": "(empty message)", "agent": selfID()})
		return
	}

	// Slash commands (/cmd …) are dispatched by the engine slash registry, NOT the
	// LLM — deterministic, reliable, independent of the model.
	// Internal: telegram-channel fetches the group→command list here (over the bus)
	// to auto-sync the Telegram slash menu. Not user-facing.
	if strings.TrimSpace(in.Text) == "/__groupcmds__" {
		emit(map[string]any{"reply": groupCommandsJSON(), "agent": selfID()})
		return
	}
	// GROUP SLASH command (/<cmd> <problem>) — discoverable in Telegram's menu, for ANY
	// owner-listed group. Checked before the generic slash handler so it isn't swallowed.
	if gid, subj, ok := stripGroupSlash(in.Text); ok {
		if subj == "" {
			c := in.Text
			if i := strings.IndexAny(c, " \n"); i >= 0 {
				c = c[:i]
			}
			emit(map[string]any{"reply": "Send me the problem 🙂\nExample:\n" + c + " how do I double revenue in 3 months with no capital?", "agent": selfID()})
			return
		}
		handleGroupChat(gid, subj, in.ChatID)
		return
	}
	if strings.HasPrefix(in.Text, "/") {
		emit(map[string]any{"reply": slashRun(in.Text), "agent": selfID()})
		return
	}

	// ALL group routing is the LLM's job — it reads the request (in ANY language) and
	// picks the right group via the ask_group tool: stock → investment, deep analysis
	// → thinking, computer control → operasi-komputer-grup. No hardcoded keyword table
	// (those were Indonesian-only and broke for global users). ask_group is terminal
	// (relayGroup), so picking a group costs ONE LLM turn and the group's answer goes
	// straight to the user, in the user's language.

	// Doctrine is SACRED and injected FIRST — the always-on anti-halu layer.
	doktrin := readWS("doktrin.md")
	persona := readWS("prompt.md")
	if persona == "" {
		persona = "Lo Mr.Flow, AI agent Flowork buat Mr.Dev. Jawab santai Bahasa Indonesia, jujur, anti-halu."
	}
	personaBlock := persona
	if ts := nowUTC(); ts != "" {
		personaBlock += "\n[WAKTU_UTC: " + ts + "]"
	}

	msgs := []any{}
	if doktrin != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": doktrin})
	}
	msgs = append(msgs, map[string]any{"role": "system", "content": personaBlock})
	mem, localN := recall(in.Text)
	if mem != "" {
		msgs = append(msgs, map[string]any{
			"role":    "system",
			"content": "[Relevant MEMORY from your brain — use if relevant, do not invent beyond this]:\n" + mem,
		})
	}
	// PRIMARY privilege: pull grounding from the 5M shared corpus. Refused (and
	// skipped) automatically if this agent is ever run at extension tier.
	shared, sharedN, sharedStatus := searchShared(in.Text)
	if shared != "" {
		msgs = append(msgs, map[string]any{
			"role":    "system",
			"content": "[REFERENCE from the shared 5M corpus — grounding material, MUST verify before claiming as fact, do not swallow raw]:\n" + shared,
		})
	}
	// Multi-turn: replay the recent conversation turns so the agent is NOT stateless
	// — it remembers what was just said, not only FTS-relevant brain hits.
	histKey := histKeyFor(in.User, in.ChatID)
	hist := histLoad(histKey)
	for _, turn := range hist {
		msgs = append(msgs, map[string]any{"role": turn.Role, "content": turn.Content})
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": in.Text})

	// Tools: offer the engine-selected set to the model and run a tool-calling loop
	// — every hop (specs, llm, run) through the single loket counter.
	specs := toolSpecs()
	reply, toolsUsed := runToolLoop(msgs, specs)

	// Remember the exchange in our own isolated brain so it survives across turns.
	if reply != "" {
		_, _ = loketCall("store.brain.add", map[string]any{
			"content": "User: " + in.Text + "\nMr.Flow: " + reply,
			"wing":    "experience",
		})
		// Append this turn to the rolling conversation buffer (recent context).
		histSave(histKey, append(hist,
			histTurn{Role: "user", Content: in.Text},
			histTurn{Role: "assistant", Content: reply}))
	}

	out := map[string]any{"reply": reply, "agent": selfID()}
	if in.Debug {
		// Transparent grounding report (off by default). Lets the owner see exactly
		// which sources the answer was grounded on — and confirms the shared-corpus
		// tier grant is live for this primary agent.
		out["_debug"] = map[string]any{
			"local_hits":    localN,
			"shared_hits":   sharedN,
			"shared_status": sharedStatus,
			"model":         model(),
			"tools_exposed": len(specs),
			"tool_calls":    toolsUsed,
		}
	}
	emit(out)
}

// ── multi-turn conversation buffer (recent turns, per user/chat) ──────────────

type histTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const maxHistTurns = 6 // keep the last 6 exchanges (12 messages)

// histKeyFor derives a stable conversation key from the user or chat id.
func histKeyFor(user string, chatID int64) string {
	k := strings.TrimSpace(user)
	if k == "" && chatID != 0 {
		k = fmt.Sprintf("%d", chatID)
	}
	if k == "" {
		k = "default"
	}
	return "hist:" + k
}

func histLoad(key string) []histTurn {
	r, err := loketCall("store.kv.get", map[string]any{"k": key})
	if err != nil {
		return nil
	}
	var s struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(r, &s) != nil || s.Value == "" {
		return nil
	}
	var turns []histTurn
	_ = json.Unmarshal([]byte(s.Value), &turns)
	return turns
}

func histSave(key string, turns []histTurn) {
	if len(turns) > maxHistTurns*2 {
		turns = turns[len(turns)-maxHistTurns*2:]
	}
	b, _ := json.Marshal(turns)
	_, _ = loketCall("store.kv.set", map[string]any{"k": key, "v": string(b)})
}

// slashRun dispatches a /command via the loket (slash.run) and returns its text.
func slashRun(text string) string {
	r, err := loketCall("slash.run", map[string]any{"text": text})
	if err != nil {
		return "[slash] " + err.Error()
	}
	var s struct {
		Result map[string]any `json:"result"`
		Error  string         `json:"error"`
	}
	if json.Unmarshal(r, &s) == nil {
		if s.Error != "" {
			return "[slash] " + s.Error
		}
		for _, k := range []string{"text", "Text"} {
			if v, ok := s.Result[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return string(r)
}

// maxToolIters bounds the tool-calling loop. Tools run sequentially (one per
// turn), so a handful of rounds covers most tasks; the bound stops a runaway.
const maxToolIters = 10

// toolSpecs fetches the OpenAI function schemas the engine exposes to us
// (tool.specs). The engine picks the set (core, anti-over-prompt); we just offer
// them. Empty on any error → the agent still answers from its own knowledge.
func toolSpecs() []json.RawMessage {
	specs := []json.RawMessage{}
	if r, err := loketCall("tool.specs", map[string]any{}); err == nil {
		var out struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if json.Unmarshal(r, &out) == nil {
			specs = out.Tools
		}
	}
	// Append the synthetic `ask_group` ORCHESTRATION tool. It is loket-native
	// (handled locally in runToolLoop, NOT via the engine tool bridge): it lets
	// Mr.Flow hand a deep-analysis job to a GROUP (a colony of ants) and weave the
	// group's synthesized answer into its reply. Offered ONLY when the owner has
	// listed groups in config (store.kv "groups") — so this is config-driven and
	// Mr.Flow can reach only the groups the owner allowed (isolation).
	if gs := availableGroups(); len(gs) > 0 {
		lines := make([]string, 0, len(gs))
		for _, g := range gs {
			lines = append(lines, "- "+g.ID+": "+g.Desc)
		}
		spec := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "ask_group",
				"description": "Delegate DEEP analysis to a GROUP (a colony of agents that work multiple viewpoints, then merged by a synthesizer). Use when the user asks for analysis that fits one of these groups:\n" + strings.Join(lines, "\n"),
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"group":   map[string]any{"type": "string", "description": "target group id (one from the list)"},
						"subject": map[string]any{"type": "string", "description": "the user's request, kept VERBATIM in their original language (do not translate) — the group answers in that language"},
					},
					"required": []string{"group", "subject"},
				},
			},
		}
		if b, err := json.Marshal(spec); err == nil {
			specs = append(specs, b)
		}
	}
	return specs
}

// groupRef is one delegable GROUP: its id + a short description for the LLM.
type groupRef struct {
	ID, Command, Desc string
	Memory            bool // conversational memory on by default; "nomem" groups (executors) skip it
}

// deriveCommand turns a group id into a clean Telegram slash command (a-z0-9_, the
// first segment) when the allowlist entry doesn't declare one explicitly.
func deriveCommand(id string) string {
	seg := id
	if i := strings.Index(seg, "-"); i > 0 {
		seg = seg[:i]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(seg) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	cmd := b.String()
	if cmd == "" {
		cmd = "group"
	}
	if len(cmd) > 32 {
		cmd = cmd[:32]
	}
	return cmd
}

// availableGroups reads which GROUPs this orchestrator may delegate to from its OWN
// config (store.kv "groups"). Each entry is "id|command|desc" (or legacy "id:desc",
// where the command is derived from the id). Config-driven: adding a group is owner
// config, never a code change, and Mr.Flow reaches ONLY the groups the owner listed.
func availableGroups() []groupRef {
	r, err := loketCall("store.kv.get", map[string]any{"k": "groups"})
	if err != nil {
		return nil
	}
	var s struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(r, &s) != nil || strings.TrimSpace(s.Value) == "" {
		return nil
	}
	var out []groupRef
	for _, part := range strings.Split(s.Value, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		g := groupRef{Memory: true}
		if strings.Contains(part, "|") {
			f := strings.SplitN(part, "|", 4)
			g.ID = strings.TrimSpace(f[0])
			if len(f) > 1 {
				g.Command = strings.TrimSpace(f[1])
			}
			if len(f) > 2 {
				g.Desc = strings.TrimSpace(f[2])
			}
			if len(f) > 3 {
				if flag := strings.ToLower(strings.TrimSpace(f[3])); flag == "nomem" || flag == "raw" {
					g.Memory = false
				}
			}
		} else { // legacy "id:desc"
			g.ID = part
			if i := strings.Index(part, ":"); i > 0 {
				g.ID = strings.TrimSpace(part[:i])
				g.Desc = strings.TrimSpace(part[i+1:])
			}
		}
		if g.ID == "" {
			continue
		}
		if g.Command == "" {
			g.Command = deriveCommand(g.ID)
		}
		out = append(out, g)
	}
	return out
}

// askGroup is the orchestrator hop: mr-flow → GROUP → member ants → synthesizer →
// back. It hands the subject to the group over the loket bus (bus.request) and
// returns the group's synthesized answer as the tool result, which the LLM then
// weaves into its final reply. Only owner-listed groups are reachable.
func askGroup(argsRaw json.RawMessage) string {
	var a struct {
		Group   string `json:"group"`
		Subject string `json:"subject"`
	}
	_ = json.Unmarshal(argsRaw, &a)
	a.Group = strings.TrimSpace(a.Group)
	a.Subject = strings.TrimSpace(a.Subject)
	if a.Group == "" || a.Subject == "" {
		return `{"error":"ask_group: group dan subject wajib"}`
	}
	ok := false
	for _, g := range availableGroups() {
		if g.ID == a.Group {
			ok = true
			break
		}
	}
	if !ok {
		return `{"error":"group tidak terdaftar di config (minta owner daftarin di kv groups)"}`
	}
	r, err := loketCall("bus.request", map[string]any{
		"to":      a.Group,
		"type":    "task",
		"payload": map[string]any{"text": a.Subject},
	})
	if err != nil {
		return `{"error":"group error: ` + jsonEsc(err.Error()) + `"}`
	}
	// bus.request → {reply:<group emit>}; the group emit → {reply:"…", …}. Unwrap.
	var outer struct {
		Reply json.RawMessage `json:"reply"`
	}
	_ = json.Unmarshal(r, &outer)
	var inner struct {
		Reply string `json:"reply"`
	}
	if json.Unmarshal(outer.Reply, &inner) == nil && inner.Reply != "" {
		// jsonStr (proper JSON marshal) preserves newlines as \n — using jsonEsc here
		// flattened them to spaces, turning a formatted answer into one wall of text.
		return `{"group_result":` + jsonStr(inner.Reply) + `}`
	}
	return string(r)
}

// fallbackTierModel — the cheap, high-rate-limit tier the orchestrator drops to
// when its primary (premium) model is throttled, so an interactive chat still gets
// a reply instead of a deadline hang. The router's fleet-wide 429 backoff is
// owner-locked and untouched; this only softens the INTERACTIVE path.
const fallbackTierModel = "claude-haiku-4-5"

// llmComplete runs llm.complete with rate-limit resilience: the primary model first,
// bounded SHORT of the response deadline; on any failure (throttle/timeout/5xx) it
// falls back ONCE to the cheap tier (rarely rate-limited). Both attempts are time-
// bounded so the pair fits inside the deadline (≈45s + ≈35s < the 90s kernel limit).
func llmComplete(llmArgs map[string]any) (json.RawMessage, error) {
	primary := model()
	llmArgs["model"] = primary
	r, lastErr := loketCallT("llm.complete", llmArgs, 45000)
	if lastErr == nil {
		return r, nil
	}
	if fallbackTierModel != "" && fallbackTierModel != primary {
		llmArgs["model"] = fallbackTierModel
		r2, err2 := loketCallT("llm.complete", llmArgs, 35000)
		if err2 == nil {
			return r2, nil
		}
		lastErr = err2
	}
	// Propagate the REAL underlying error (not a generic "rate-limited") so the caller
	// can tell a dead gateway apart from an actual throttle — see llmFailMessage.
	return nil, lastErr
}

// llmFailMessage turns an LLM-call failure into an honest user-facing line. It must
// NOT cry "penuh/limit" for a dead gateway (connection refused) — that mislabel cost
// hours of false "rate-limit" debugging. The error propagates from the host as
// "loket refused: llm router: ...connection refused" (gateway down) vs "llm: ...429..."
// (real throttle); classify on that text.
func llmFailMessage(err error) string {
	if err == nil {
		return "⏳ Lagi ada kendala bro, coba lagi sebentar ya 🙏"
	}
	e := strings.ToLower(err.Error())
	switch {
	case strings.Contains(e, "connection refused"), strings.Contains(e, "dial tcp"),
		strings.Contains(e, "no such host"), strings.Contains(e, "connect: connection"),
		strings.Contains(e, "no response after retries"):
		return "⚠️ Gateway-nya kayaknya lagi mati bro (koneksi ke service gagal, bukan limit) — coba cek service-nya jalan dulu."
	case strings.Contains(e, "429"), strings.Contains(e, "rate"), strings.Contains(e, "too many"),
		strings.Contains(e, "overloaded"), strings.Contains(e, "quota"), strings.Contains(e, "throttl"):
		return "⏳ Modelnya lagi penuh/limit bro, coba lagi sebentar ya 🙏"
	default:
		return "⏳ Lagi ada kendala manggil model bro, coba lagi sebentar ya 🙏"
	}
}

// runToolLoop is Mr.Flow's tool-calling loop, every hop through the loket: offer
// tools → the model asks for one → tool.run executes it → feed the result back →
// repeat until the model answers in plain text. Returns the final reply + how
// many tools ran.
//
// It mirrors the legacy agent's hard-won rules: process only the FIRST tool_call
// per turn (the router mistranslates parallel tool_results into a 400), keep the
// assistant content non-empty (some providers reject empty content alongside
// tool_calls), and pair each tool_call id with exactly one tool result.
func runToolLoop(msgs []any, specs []json.RawMessage) (string, int) {
	toolsUsed := 0
	for iter := 0; iter < maxToolIters; iter++ {
		llmArgs := map[string]any{"messages": msgs}
		if len(specs) > 0 {
			llmArgs["tools"] = specs
			llmArgs["parallel_tool_calls"] = false
		}
		r, err := llmComplete(llmArgs)
		if err != nil {
			// Rate-limit / timeout even after the cheap-tier fallback → fail SOFT with a
			// clean message, never a raw error or (worse) a silent deadline hang.
			return llmFailMessage(err), toolsUsed
		}
		var resp struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		}
		_ = json.Unmarshal(r, &resp)
		if len(resp.ToolCalls) == 0 {
			return strings.TrimSpace(resp.Content), toolsUsed // final text answer
		}
		tc := resp.ToolCalls[0] // serialize: one tool per turn
		id := fmt.Sprintf("call_%d", iter)
		content := resp.Content
		if strings.TrimSpace(content) == "" {
			content = "(memanggil tool)"
		}
		msgs = append(msgs, map[string]any{
			"role": "assistant", "content": content,
			"tool_calls": []any{map[string]any{
				"id": id, "type": "function",
				"function": map[string]any{"name": tc.Function.Name, "arguments": tc.Function.Arguments},
			}},
		})
		argsRaw := json.RawMessage("{}")
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			argsRaw = json.RawMessage(tc.Function.Arguments)
		}
		// ask_group is a TERMINAL delegation: the LLM PICKS the right group (that is
		// the intelligence — no hardcoded keyword routing), but the group's synthesizer
		// has already produced a complete, user-facing answer IN THE USER'S LANGUAGE, so
		// we relay it DIRECTLY instead of paying a second LLM turn to rewrite it. That
		// second turn would also risk the response deadline on a long multi-organ
		// pipeline (the reason the old deterministic pre-routers existed). LLM-as-router,
		// group-as-answer. Every other tool loops back so the LLM can use its result.
		if tc.Function.Name == "ask_group" {
			toolsUsed++
			return relayGroup(askGroup(argsRaw)), toolsUsed
		}
		result := toolRun(tc.Function.Name, argsRaw)
		toolsUsed++
		msgs = append(msgs, map[string]any{
			"role": "tool", "tool_call_id": id, "content": result,
		})
	}
	return "(batas loop tool kena — coba perjelas permintaan lo)", toolsUsed
}

// relayGroup turns an ask_group result envelope into the final user-facing reply:
// the group's synthesized answer, markdown-stripped for Telegram. A single-executor
// group (no synthesizer) labels its one section "### <id> …" — strip that noise.
func relayGroup(raw string) string {
	var gr struct {
		GroupResult string `json:"group_result"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal([]byte(raw), &gr)
	reply := strings.TrimSpace(gr.GroupResult)
	if reply == "" {
		if gr.Error != "" {
			return "Grup error: " + gr.Error
		}
		return "Grup belum ngasih jawaban — coba lagi bentar ya."
	}
	if strings.HasPrefix(reply, "### ") {
		rest := reply[len("### "):]
		if i := strings.IndexAny(rest, " \n"); i >= 0 {
			reply = strings.TrimSpace(rest[i+1:])
		}
	}
	return plainify(reply)
}

// toolRun executes one tool by name via the loket bridge (tool.run) and returns
// its raw result JSON, fed straight back to the model as the tool reply.
func toolRun(name string, argsRaw json.RawMessage) string {
	r, err := loketCall("tool.run", map[string]any{"name": name, "args": argsRaw})
	if err != nil {
		return `{"error":"tool.run refused/failed: ` + jsonEsc(err.Error()) + `"}`
	}
	return string(r)
}

// jsonEsc minimally escapes a string for inlining inside a JSON error literal.
func jsonEsc(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// jsonStr marshals a string to a JSON string literal (with quotes).
func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

// plainify strips markdown to clean chat text (Telegram renders no markdown, so
// "##" / "**" / "---" would show raw). Done in code because the model keeps using
// markdown even when told not to. Preserves newlines + simple "-" bullets.
func plainify(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimRight(ln, " \t")
		trimmed := strings.TrimSpace(t)
		// drop horizontal rules (---, ***, ___ of any length)
		if len(trimmed) >= 3 && (strings.Trim(trimmed, "-") == "" || strings.Trim(trimmed, "*") == "" || strings.Trim(trimmed, "_") == "") {
			out = append(out, "")
			continue
		}
		// header lines (#, ##, …) → plain line
		ls := strings.TrimLeft(t, " ")
		if strings.HasPrefix(ls, "#") {
			t = strings.TrimSpace(strings.TrimLeft(ls, "#"))
		} else if strings.HasPrefix(ls, "* ") || strings.HasPrefix(ls, "+ ") {
			indent := t[:len(t)-len(ls)]
			t = indent + "- " + ls[2:]
		}
		out = append(out, t)
	}
	res := strings.Join(out, "\n")
	for strings.Contains(res, "\n\n\n") {
		res = strings.ReplaceAll(res, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(res)
}

// tkvGet/tkvSet — small kv helpers for the thinking conversation memory (own store).
func tkvGet(k string) string {
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
func tkvSet(k, v string) { _, _ = loketCall("store.kv.set", map[string]any{"k": k, "v": v}) }

// stripGroupSlash recognizes a GROUP slash command (/<command> <problem>, with the
// optional @botname form in groups) and maps it to the group id — for ANY group the
// owner listed. So every group gets a discoverable command automatically; no per-group
// code. Returns (groupID, problem, ok).
func stripGroupSlash(text string) (groupID, subject string, ok bool) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		return "", "", false
	}
	cmd, rest := t, ""
	if i := strings.IndexAny(t, " \n"); i >= 0 {
		cmd, rest = t[:i], strings.TrimSpace(t[i+1:])
	}
	if i := strings.Index(cmd, "@"); i >= 0 { // strip @botname (groups)
		cmd = cmd[:i]
	}
	c := strings.ToLower(strings.TrimPrefix(cmd, "/"))
	if c == "think" || c == "pikir" || c == "mikir" { // friendly aliases for thinking
		c = "thinking"
	}
	for _, g := range availableGroups() {
		if strings.ToLower(g.Command) == c {
			return g.ID, rest, true
		}
	}
	return "", "", false
}

// groupCommandsJSON returns the owner's groups as a Telegram setMyCommands payload —
// fetched by the telegram-channel (over the bus) so the slash menu auto-syncs with
// whatever groups exist. No shared store needed (respects isolation).
func groupCommandsJSON() string {
	type cmd struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	cs := []cmd{}
	for _, g := range availableGroups() {
		d := strings.TrimSpace(g.Desc)
		if d == "" {
			d = "Group " + g.ID
		}
		if len(d) > 240 {
			d = d[:240]
		}
		cs = append(cs, cmd{Command: g.Command, Description: d})
	}
	b, _ := json.Marshal(map[string]any{"commands": cs})
	return string(b)
}

// handleThinking runs the thinking colony for one user message, with per-chat
// rolling memory so it can continue a multi-turn diagnosis. Used by BOTH the slash
// command and the legacy keyword trigger.
func handleGroupChat(groupID, userMsg string, chatID int64) {
	userMsg = strings.TrimSpace(userMsg)
	mem := true
	for _, g := range availableGroups() {
		if g.ID == groupID {
			mem = g.Memory
			break
		}
	}
	histKey := "ghist:" + groupID + ":" + strconv.FormatInt(chatID, 10)
	hist := ""
	if mem {
		if low := strings.ToLower(userMsg); strings.Contains(low, "topik baru") || strings.Contains(low, "mulai baru") || strings.Contains(low, "reset") {
			tkvSet(histKey, "") // explicit fresh start
		}
		hist = tkvGet(histKey)
	}
	subject := userMsg
	if mem && strings.TrimSpace(hist) != "" {
		subject = "Earlier conversation context (CONTINUE from here, do not start over):\n" +
			hist + "=== Latest user message ===\n" + userMsg
	}
	res := askGroup(json.RawMessage(`{"group":` + jsonStr(groupID) + `,"subject":` + jsonStr(subject) + `}`))
	reply := ""
	var gr struct {
		GroupResult string `json:"group_result"`
		Error       string `json:"error"`
	}
	if json.Unmarshal([]byte(res), &gr) == nil {
		if gr.GroupResult != "" {
			reply = gr.GroupResult
		} else if gr.Error != "" {
			reply = "group error: " + gr.Error
		}
	}
	if strings.TrimSpace(reply) == "" {
		reply = "The group returned no answer."
	} else {
		reply = plainify(reply)
	}
	if mem && !strings.HasPrefix(reply, "group error") && !strings.HasPrefix(reply, "The group returned no answer") {
		ans := reply
		if len(ans) > 700 {
			ans = ans[:700] + " …"
		}
		newHist := hist + "User: " + userMsg + "\nTim: " + ans + "\n\n"
		newHist = compactHist(newHist)
		tkvSet(histKey, newHist)
	}
	emit(map[string]any{"reply": reply, "agent": selfID()})
}

// compactHist (P4 — context compaction): keep the rolling group-chat buffer bounded
// WITHOUT a blind mid-content char cut. When it overflows, keep the newest turns
// verbatim (on clean "\n\n" turn boundaries) and fold the older turns into a one-line
// summary memo, so continuity survives. Only fires on overflow; the summary uses the
// resilient llmComplete and degrades to a clean boundary drop if the model is busy.
func compactHist(history string) string {
	const capChars, keepChars = 2400, 1600
	if len(history) <= capChars {
		return history
	}
	turns := strings.Split(strings.TrimRight(history, "\n"), "\n\n")
	keep := []string{}
	total := 0
	for i := len(turns) - 1; i >= 0; i-- {
		if total+len(turns[i]) > keepChars && len(keep) > 0 {
			old := strings.Join(turns[:i+1], "\n\n")
			head := ""
			if s := summarizeHist(old); s != "" {
				head = "[ringkasan percakapan awal: " + s + "]\n\n"
			}
			return head + strings.Join(keep, "\n\n") + "\n\n"
		}
		keep = append([]string{turns[i]}, keep...) // prepend → chronological
		total += len(turns[i])
	}
	return history
}

// summarizeHist compresses older turns into ONE memo line via the resilient LLM path.
// "" on failure → caller degrades to a clean-boundary drop (never blocks the reply).
func summarizeHist(old string) string {
	r, err := llmComplete(map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "Ringkas percakapan ini jadi SATU kalimat memo — fakta + keputusan penting saja, tanpa basa-basi. Bahasa ikut percakapan."},
		map[string]any{"role": "user", "content": old},
	}})
	if err != nil {
		return ""
	}
	var resp struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(r, &resp) != nil {
		return ""
	}
	s := strings.TrimSpace(resp.Content)
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
