// === LOCKED FILE ===
// Status: STABLE — `thinking` group lens (RAG ant). Tested end-to-end 2026-06-08:
//   grounded retrieval (recalled>0) bilingual EN/ID, anti-halu refusal verified.
// Do not edit without owner approval. Rebuild: GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
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
	if r, err := loketCall("store.brain.search", map[string]any{"query": in.Text, "k": 8}); err == nil {
		var s struct {
			Hits []struct {
				Content string `json:"content"`
			} `json:"hits"`
		}
		if json.Unmarshal(r, &s) == nil {
			for _, h := range s.Hits {
				if c := strings.TrimSpace(h.Content); c != "" {
					recalled = append(recalled, c)
				}
			}
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

	emit(map[string]any{"reply": reply, "agent": selfID(), "recalled": len(recalled)})
}
