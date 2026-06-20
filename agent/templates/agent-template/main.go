// Package main — TEMPLATE AGENT FLOWORK (worker generic, "pasukan semut").
//
// Ini CETAKAN agent baru sesuai STANDAR Flowork (lihat AGENT_STANDARD.md +
// ../../agents/readme.md). Prinsip: "agent bodoh, engine pinter" — wasm SAMA jadi
// agent beda cukup ganti manifest.id + config (persona DB), TANPA edit kode.
//
// STANDAR yang DIPATUHI di sini:
//   - Persona DB-BASED (FLOWORK_AGENT_CONFIG, di-inject host dari config agent) —
//     BUKAN file .md. GUI = kebenaran; edit persona di GUI langsung kepakai.
//   - Akses dunia lewat SATU pintu loket (call(cap,args) ke /api/kernel/call) —
//     kapabilitas digate broker (manifest.capabilities_required).
//   - Brain dua-lapis: lokal (store.brain.*) + shared router (brain.search.shared)
//     lewat genome pipe (auto dari host coreExposedTools).
//   - DNA (konstitusi sacred + cognitive graph) di-seed HOST via ProvisionAgentDNA
//     pas boot/install — TIDAK perlu di-kode di wasm ini.
//
// Build: GOWORK=off GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .
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

// loketCall — SATU pintu agent ke dunia: minta kapabilitas ke kernel by-name.
// Balik field "result" kalau sukses; error kalau broker nolak (cap ga di-grant)
// atau provider gagal. Host inject id + loopback-secret tiap request (un-forgeable).
func loketCall(capName string, args any) (json.RawMessage, error) {
	argsJSON, _ := json.Marshal(args)
	body, _ := json.Marshal(map[string]any{"cap": capName, "args": json.RawMessage(argsJSON)})
	reqJSON, _ := json.Marshal(map[string]any{
		"method": "POST", "url": loketURL,
		"timeout_ms": 240000, "max_resp_bytes": 4 << 20,
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
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch os.Args[1] {
	case "handle_message": // RPC/direct: args = payload {"text":...}
		handleMessage(args)
	case "handle": // loket-bus (group route): unwrap payload
		var msg struct {
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal([]byte(args), &msg)
		if len(msg.Payload) == 0 {
			msg.Payload = json.RawMessage(args)
		}
		handleMessage(string(msg.Payload))
	case "boot":
		emit(map[string]any{"ok": true}) // worker = no daemon (orchestrator yg punya daemon)
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

// agentConfig — persona + model, di-inject host dari config DB agent
// (FLOWORK_AGENT_CONFIG). INI resep "copas": wasm sama → agent beda cukup ganti
// config. GUI = kebenaran (edit persona di GUI menang). TIDAK baca .md.
type agentConfig struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func loadConfig() agentConfig {
	c := agentConfig{
		Prompt: "Lo agent spesialis Flowork. Kerjain SATU tugas dengan jelas + jujur (anti-halu). Persona ini placeholder — ganti di GUI/config.",
		Model:  "flowork-brain",
	}
	if raw := os.Getenv("FLOWORK_AGENT_CONFIG"); raw != "" {
		var p agentConfig
		if json.Unmarshal([]byte(raw), &p) == nil {
			if p.Prompt != "" {
				c.Prompt = p.Prompt
			}
			if p.Model != "" {
				c.Model = p.Model
			}
		}
	}
	return c
}

// handleMessage — tugas worker: ingat pesan (brain lokal) → recall (lokal + shared
// router via genome) → jawab LLM → catat pengalaman. Konstitusi sacred (anti-halu/
// sync-honest/autonomy-mode) di-inject HOST ke konteks (DNA), ga perlu hardcode.
func handleMessage(argsJSON string) {
	var in struct {
		Text string `json:"text"`
		User string `json:"user"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &in)
	if strings.TrimSpace(in.Text) == "" {
		emit(map[string]any{"reply": "(pesan kosong)"})
		return
	}
	cfg := loadConfig()

	// 1. Ingat pesan ke brain LOKAL (terisolasi).
	_, _ = loketCall("store.brain.add", map[string]any{"content": in.Text, "wing": "experience"})

	// 2. Recall relevan — lokal dulu, lalu shared router (genome) buat ground jawaban.
	ctx := ""
	if r, err := loketCall("store.brain.search", map[string]any{"query": in.Text, "k": 3}); err == nil {
		ctx += "[recall-lokal] " + trunc(string(r), 600) + "\n"
	}
	if r, err := loketCall("brain.search.shared", map[string]any{"query": in.Text, "k": 3}); err == nil {
		ctx += "[recall-shared] " + trunc(string(r), 600) + "\n"
	}

	// 3. Jawab LLM (persona DB + konteks recall). Konstitusi/DNA di-inject host.
	msgs := []map[string]string{{"role": "system", "content": cfg.Prompt}}
	if ctx != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": "GROUNDING (recall brain, pakai kalau relevan; jangan halu):\n" + ctx})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": in.Text})

	reply := ""
	if r, err := loketCall("llm.complete", map[string]any{"model": cfg.Model, "messages": msgs}); err == nil {
		var s struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(r, &s)
		reply = strings.TrimSpace(s.Content)
		learn("experience", "job", "Did: "+trunc(in.Text, 200)+"\n→ "+trunc(reply, 400))
	} else {
		reply = fmt.Sprintf("[%s] pesan ke-ingat, tapi LLM offline (%s).", selfID(), trunc(err.Error(), 120))
		learn("experience", "mistake", "LLM gagal buat: "+trunc(in.Text, 200)+" — "+trunc(err.Error(), 160))
	}
	emit(map[string]any{"reply": reply, "agent": selfID()})
}

// learn — tulis drawer ke brain LOKAL (lapis bawah brain dua-lapis; router pegang
// brain shared). wing=kategori (experience/eureka), room=tag halus (job/mistake).
func learn(wing, room, content string) {
	_, _ = loketCall("store.brain.add", map[string]any{"content": content, "wing": wing, "room": room})
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
