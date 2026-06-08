// === LOCKED FILE ===
// Status: STABLE — `thinking` group lens (RAG ant). Tested 2026-06-08: grounded retrieval
//   bilingual EN/ID + anti-halu refusal; PATTERN self-wiring (item 12/14/15): recalled
//   patterns wire on success (kv adjacency, pruned), recall pulls strong neighbors
//   (spreading-activation). Do not edit without owner approval.
//   Rebuild: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
//
// Package main is the Flowork "lens" template — a grounded (RAG) ant.
//
// It is a thin variant of the ant template for the "thinking" group: a tiny
// specialist that reasons about a subject through ONE fixed lens (a way of
// thinking). The difference from the plain ant: before it answers, it RETRIEVES
// the matching principles from its OWN isolated brain (store.brain.search) and
// injects them into the prompt as the ONLY allowed ground. This is what keeps it
// from hallucinating: it speaks from ingested patterns, not from the model's
// free imagination. If the brain returns nothing, it must say so — never invent.
//
// The lens is read-only over its doctrine brain: it does NOT write the incoming
// question back into the brain, so the doctrine corpus stays pure (no user text
// leaking in and resurfacing later as if it were a principle).
//
// Same "copas" recipe as the ant: the SAME wasm becomes a different lens just by
// changing prompt.md (the persona) + the brain you ingest into it — no code edit.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unsafe"
)

// readWS reads a file from this lens's OWN folder (mounted at /workspace):
// prompt.md (who the lens is) and doktrin.md (sacred anti-halu rules). "" if absent.
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

func emit(v any) {
	b, _ := json.Marshal(v)
	fmt.Println(string(b))
}

func selfID() string { return os.Getenv("FLOWORK_AGENT_ID") }

const loketURL = "http://127.0.0.1:1987/api/kernel/call"

// loketCall is the lens's ONLY door to the world: ask the kernel for a capability
// by name. Returns the raw "result" on success, or an error if refused/failed.
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
		handleMessage(args)
	case "handle":
		// Loket-bus invocation: unwrap the Message payload so a group can route
		// work to this lens the same way a chat does.
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) == 0 {
			msg.Payload = json.RawMessage(args)
		}
		handleMessage(string(msg.Payload))
	case "boot":
		emit(map[string]any{"ok": true})
	default:
		emit(map[string]any{"error": "unknown function: " + fn})
	}
}

// agentConfig is the lens's persona + model, injected by the host. Persona is
// normally read from prompt.md (transparent file); config is the fallback.
type agentConfig struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func loadConfig() agentConfig {
	c := agentConfig{
		Prompt: "You reason about a subject through one fixed lens, grounded only in retrieved principles.",
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

// --- pattern self-wiring (ROADMAP_THINKING.md item 12/14/15) ---
// Edges between PATTERNS, stored in kv adjacency lists (the loket brain store is frozen,
// so we wire on top of kv). "Firing creates wiring": patterns recalled together for a
// real answer get a stronger edge; a later recall pulls a pattern's strongest neighbors
// into the grounding (spreading-activation). Bounded (top-3 core) + pruned (top-6 keep).

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
func kvSet(k, v string) { _, _ = loketCall("store.kv.set", map[string]any{"k": k, "v": v}) }

// patternID is a stable id for a pattern's text (keys its adjacency list).
func patternID(s string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(h[:])[:16]
}

type neighbor struct {
	ID      string `json:"id"`
	Content string `json:"c"`
	Weight  int    `json:"w"`
}

func neighbors(id string) []neighbor {
	raw := kvGet("adj:" + id)
	if raw == "" {
		return nil
	}
	var ns []neighbor
	_ = json.Unmarshal([]byte(raw), &ns)
	return ns
}

// writeNeighbors prunes to the strongest 6 (item 14: decay/prune) before storing.
func writeNeighbors(id string, ns []neighbor) {
	sort.Slice(ns, func(i, j int) bool { return ns[i].Weight > ns[j].Weight })
	if len(ns) > 6 {
		ns = ns[:6]
	}
	b, _ := json.Marshal(ns)
	kvSet("adj:"+id, string(b))
}

func upsertNeighbor(ns []neighbor, id, content string) []neighbor {
	for i := range ns {
		if ns[i].ID == id {
			ns[i].Weight++
			return ns
		}
	}
	if len(content) > 200 {
		content = content[:200]
	}
	return append(ns, neighbor{ID: id, Content: content, Weight: 1})
}

// addEdge strengthens the undirected edge between two patterns (both adjacency lists).
func addEdge(aID, aContent, bID, bContent string) {
	if aID == bID {
		return
	}
	writeNeighbors(aID, upsertNeighbor(neighbors(aID), bID, bContent))
	writeNeighbors(bID, upsertNeighbor(neighbors(bID), aID, aContent))
}

// handleMessage is the lens's single job: RETRIEVE matching principles from its
// own brain, then answer the subject grounded ONLY in those principles.
func handleMessage(argsJSON string) {
	var in struct {
		Text string `json:"text"`
		User string `json:"user"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	if strings.TrimSpace(in.Text) == "" {
		emit(map[string]any{"reply": "(empty message)"})
		return
	}

	// 1. Retrieve grounded principles from this lens's OWN isolated brain.
	var recalled []string
	var core []string // top originally-recalled patterns (used for wiring on success)
	seen := map[string]bool{}
	if r, err := loketCall("store.brain.search", map[string]any{"query": in.Text, "k": 6}); err == nil {
		var s struct {
			Hits []struct {
				Content string `json:"content"`
			} `json:"hits"`
		}
		if json.Unmarshal(r, &s) == nil {
			for _, h := range s.Hits {
				c := strings.TrimSpace(h.Content)
				if c == "" || seen[c] {
					continue
				}
				seen[c] = true
				recalled = append(recalled, c)
				if len(core) < 3 {
					core = append(core, c)
				}
			}
		}
	}

	// 1b. SPREADING-ACTIVATION (item 15): pull each core pattern's strongest wired
	//     neighbors into the grounding, so the lens reasons "in connections" (firing
	//     spreads through synapses), not just in isolated drawers. Capped to +3.
	spread := 0
	for _, c := range core {
		if spread >= 3 {
			break
		}
		for _, nb := range neighbors(patternID(c)) {
			if spread >= 3 {
				break
			}
			if nb.Content == "" || seen[nb.Content] {
				continue
			}
			seen[nb.Content] = true
			recalled = append(recalled, nb.Content)
			spread++
		}
	}

	// 2. Build the prompt: doctrine (anti-halu) + persona + retrieved ground.
	cfg := loadConfig()
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
	if len(recalled) > 0 {
		var b strings.Builder
		b.WriteString("Principles retrieved from your own knowledge base. Ground your whole answer ONLY in these — do not add ideas that are not supported here:\n")
		for _, c := range recalled {
			b.WriteString("- ")
			b.WriteString(c)
			b.WriteString("\n")
		}
		msgs = append(msgs, map[string]string{"role": "system", "content": b.String()})
	} else {
		msgs = append(msgs, map[string]string{"role": "system", "content": "Your knowledge base returned nothing relevant to this subject. Say plainly that you have no grounded basis for it, and do NOT invent an answer."})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": in.Text})

	// 3. Ask the small model.
	reply := ""
	if r, err := loketCall("llm.complete", map[string]any{"model": cfg.Model, "messages": msgs}); err == nil {
		var s struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(r, &s)
		reply = s.Content
	} else {
		reply = "[lens] LLM offline: " + err.Error()
	}

	// WIRE ON SUCCESS (item 11/13): a real grounded answer means the core patterns
	// co-activated usefully → strengthen their pairwise edges (bounded to the top-3 core,
	// pruned in writeNeighbors). Edges from a real answer, never from a guess.
	if strings.TrimSpace(reply) != "" && !strings.HasPrefix(reply, "[lens] LLM offline") && len(core) >= 2 {
		for i := 0; i < len(core); i++ {
			for j := i + 1; j < len(core); j++ {
				addEdge(patternID(core[i]), core[i], patternID(core[j]), core[j])
			}
		}
	}
	emit(map[string]any{"reply": reply, "agent": selfID(), "recalled": len(recalled), "spread": spread, "wired": len(core)})
}
