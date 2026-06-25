// Package agentkit — KIT BERSAMA "pasukan semut" Flowork (keystone roadmap AGENTKIT).
//
// ⚠️ FROZEN (chattr +i + hash KERNEL_FREEZE.md, owner-approved 2026-06-25). Inti loop SEMUA
// worker-semut → edit = rebuild SEMUA agent. Ubah = SADAR: unfreeze → edit → rebuild tiap
// agent → re-hash KERNEL_FREEZE → re-freeze → verify Rule-9. Arsitektur: lock/agentkit.md.
//
// AKAR (Rule 5/6, cabut-akar bukan tambal): loop tool-calling tiap worker-agent dulu
// KE-COPY 6× (mr-flow + 5 agent + template). Guard deterministik (flail/ghost) cuma di
// mr-flow → 5 agent lain GAK punya → nyalain all-tools/#2C buat mereka = FLAIL. Solusi:
// EKSTRAK loop+guard jadi 1 package SHARED → tiap main.go agent jadi bootstrap TIPIS
// (`func main(){ agentkit.Main() }`) → SEMUA semut + template warisan. Fix sekali → semua
// dapet. Ini yang bikin all-tools GLOBAL + #2C semua-semut MUNGKIN (bukan cuma mr-flow).
//
// CAKUPAN: ini loop WORKER (autonomus, persona DB + DNA self-prompt, tool dari subscription/
// all-tools). mr-flow PUNYA loop sendiri (brain-heavy: Telegram I/O, media, recall, history,
// working-set) — TETAP frozen & TERPISAH (referensi, sengaja TIDAK di-migrate: dia bukan
// akar duplikasi worker + bukak jantungnya = risiko tanpa untung). Hasil: 6 copy → 2
// implementasi (agentkit utk semua semut + mr-flow brain).
//
// Guard di guards.go: flail-guard (anti-mantok), ghost-guard (anti-ghosting), recovery-
// capture (self-learning). Anti-anchor TIDAK dipasang di sini: loop worker rakit msgs FRESH
// tiap turn (ga feed history balik) → ga ada regurgitasi-history. Kalau kelak worker dikasih
// history, anti-anchor nyusul di sini.
//
// TinyGo-safe: no regexp, no goroutine (-scheduler=none), stdlib minimal. Owner: Aola Sahidin.
package agentkit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

//go:wasmimport flowork host_time_now_ms
func hostTimeNowMs() uint64

var outBuf [1 << 20]byte // 1MB: muat tool-specs + response LLM

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func emit(v any)     { b, _ := json.Marshal(v); fmt.Println(string(b)) }
func selfID() string { return os.Getenv("FLOWORK_AGENT_ID") }

const (
	routerURL = "http://127.0.0.1:2402/v1/chat/completions"
	specsURL  = "http://127.0.0.1:1987/api/agents/tools/specs?id="
	toolRun   = "http://127.0.0.1:1987/api/agents/tools/run?id="
	selfPromp = "http://127.0.0.1:1987/api/agents/self-prompt/render?id="

	maxToolIters           = 100    // backstop anti pure-infinite; batas NYATA = loopBudgetMs
	loopBudgetMs    uint64 = 200000 // budget waktu in-turn (~200s); turn-timeout 290s = backstop keras
	maxGhostNudges         = 6      // ghost-guard: max paksa-lanjut pas narasi-tanpa-tool (bounded)
	maxAutoContinue        = 50     // anti-runaway: max chunk auto-continue (≈2.7 jam) — BUKAN batas kerja
	autoContDelim          = "\n===TUGAS===\n"
)

type httpResp struct {
	Status int
	Body   []byte
}

func fetch(method, url string, headers map[string]string, body []byte, timeoutMS int) (*httpResp, error) {
	// #3 scoped-instinct (RI-5): self-identify ke router HANYA pada call LLM (chat/completions)
	// → router bisa scope insting by-peran. No-op kalau selfID kosong. Header ekstra = harmless.
	if id := selfID(); id != "" && strings.Contains(url, "/v1/chat/completions") {
		if headers == nil {
			headers = map[string]string{}
		}
		headers["X-Agent-ID"] = id
	}
	req := map[string]any{"method": method, "url": url, "timeout_ms": timeoutMS, "max_resp_bytes": 4 << 20}
	if len(headers) > 0 {
		req["headers"] = headers
	}
	if len(body) > 0 {
		req["body_base64"] = base64.StdEncoding.EncodeToString(body)
	}
	reqJSON, _ := json.Marshal(req)
	n := hostNetFetch(bytesPtr(reqJSON), uint32(len(reqJSON)), bytesPtr(outBuf[:]), uint32(len(outBuf)))
	if n == 0 {
		return nil, fmt.Errorf("host_net_fetch 0 bytes")
	}
	var hr struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:n], &hr); err != nil {
		return nil, err
	}
	if hr.Error != "" {
		return nil, fmt.Errorf("host: %s", hr.Error)
	}
	b, _ := base64.StdEncoding.DecodeString(hr.BodyB64)
	return &httpResp{Status: hr.Status, Body: b}, nil
}

// Main — entry-point bootstrap tipis untuk SEMUA worker-agent. main.go agent cukup:
//
//	package main
//	import "flowork-gui/internal/agentkit"
//	func main() { agentkit.Main() }
//
// Dispatch os.Args sama persis kayak template lama (handle_message / handle / boot).
func Main() {
	if len(os.Args) < 2 {
		return
	}
	args := "{}"
	if len(os.Args) > 2 && os.Args[2] != "" {
		args = os.Args[2]
	}
	switch os.Args[1] {
	case "handle_message":
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
		emit(map[string]any{"ok": true}) // worker = no daemon
	default:
		emit(map[string]any{"error": "unknown function: " + os.Args[1]})
	}
}

type agentConfig struct {
	Prompt string
	Model  string
}

// loadConfig — baca persona + router.model dari GUI (FLOWORK_AGENT_CONFIG). GUI = SUMBER
// KEBENARAN (owner 2026-06-20). model di router.model (nested), BUKAN top-level "model"
// (bug lama → semua agent jatuh ke flowork-brain). Hormati top-level "model" sbg back-compat.
func loadConfig() agentConfig {
	c := agentConfig{
		Prompt: "Lo agent spesialis Flowork. Kerjain tugas dengan jelas + jujur (anti-halu). Ganti persona ini di GUI/config.",
		Model:  "flowork-brain", // last-resort ONLY kalau GUI belum set model
	}
	if raw := os.Getenv("FLOWORK_AGENT_CONFIG"); raw != "" {
		var p struct {
			Prompt string `json:"prompt"`
			Router struct {
				Model string `json:"model"`
			} `json:"router"`
			Model string `json:"model"`
		}
		if json.Unmarshal([]byte(raw), &p) == nil {
			if p.Prompt != "" {
				c.Prompt = p.Prompt
			}
			if p.Router.Model != "" {
				c.Model = p.Router.Model
			} else if p.Model != "" {
				c.Model = p.Model
			}
		}
	}
	return c
}

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
	emit(map[string]any{"reply": callLLM(loadConfig(), in.Text), "agent": selfID()})
}

// callRouterRetry — resilience: panggil router LLM dgn RETRY exp-backoff buat error
// TRANSIENT (jaringan/timeout/5xx/429/408), TRANSPARAN ke loop. Fatal (4xx selain
// 429/408) → balik langsung, NO retry. Switch FLOWORK_ROUTER_RETRY (default 5).
func callRouterRetry(reqJSON []byte) (*httpResp, error) {
	maxAtt := 5
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ROUTER_RETRY")); v != "" {
		if n := atoiSafe(v); n >= 1 {
			maxAtt = n
		}
	}
	hdr := map[string]string{"Content-Type": "application/json"}
	var lastErr error
	for att := 1; att <= maxAtt; att++ {
		resp, err := fetch("POST", routerURL, hdr, reqJSON, 240000)
		if err == nil && resp != nil && resp.Status < 400 {
			return resp, nil
		}
		transient := err != nil || resp == nil || resp.Status >= 500 || resp.Status == 429 || resp.Status == 408
		if !transient {
			return resp, fmt.Errorf("router fatal HTTP %d (ga di-retry)", resp.Status)
		}
		lastErr = err
		if att >= maxAtt {
			break
		}
		d := 500 << uint(att-1)
		if d > 30000 {
			d = 30000
		}
		jitter := int(hostTimeNowMs() % uint64(d/4+1))
		time.Sleep(time.Duration(d+jitter) * time.Millisecond)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("router ga nyambung setelah %d retry: %v", maxAtt, lastErr)
	}
	return nil, fmt.Errorf("router error (5xx) setelah %d retry", maxAtt)
}

// atoiSafe — parse int sederhana (TinyGo/wasip1-safe). Balik -1 kalau invalid.
func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// callLLM — TOOL-LOOP autonomus (inti standar Flowork). Persona DB + DNA (self-prompt
// render) jadi system; tools dari subscription / all-tools. Loop: LLM → tool → feed →
// ulang, dengan GHOST-GUARD + FLAIL-GUARD + #2C seam + recovery + time-bound + auto-continue.
func callLLM(cfg agentConfig, userText string) string {
	sys := cfg.Prompt
	if dna := fetchSelfPrompt(); dna != "" { // konstitusi sacred + doktrin (anti-halu/sync-honest/autonomy-mode)
		sys = sys + "\n\n" + dna
	}
	msgs := []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": userText},
	}
	toolSpecs := fetchToolSpecs()
	ghostNudges := 0
	flail := &flailState{}            // flail-guard: anti-mantok (tool sama berulang tanpa progress)
	recovered := map[string]string{}  // recovery-capture: tool→kelas-error yg BELUM ke-recover (scope turn)
	loopStartMs := hostTimeNowMs()
	budgetNudged := false

	for iter := 0; iter < maxToolIters; iter++ {
		// AUTO-CONTINUE deterministik: budget abis & belum kelar → harness jadwalin
		// lanjutan sendiri (ScheduleWakeup), nyambung lintas-turn = unbounded over time.
		if !budgetNudged && hostTimeNowMs()-loopStartMs > loopBudgetMs {
			budgetNudged = true
			base, cont := parseAutoCont(userText)
			next := cont + 1
			if next > maxAutoContinue {
				return fmt.Sprintf("⏳ Udah %d kali nyambung otomatis tapi belum kelar — kemungkinan tugasnya kegedean/muter. Gw STOP biar ga infinite. Pecah jadi lebih kecil ya.", cont)
			}
			contPrompt := fmt.Sprintf("[LANJUTAN OTOMATIS #%d] Lo lagi di tengah ngerjain tugas ini & BELUM kelar. LANJUTIN dari progres terakhir (cek memori/hasil sebelumnya — JANGAN ulang dari nol, JANGAN ngaku kelar kalau belum). Pas beneran kelar, jawab hasil FINAL + tutup 'SELESAI'.%s%s", next, autoContDelim, base)
			_ = runTool("ScheduleWakeup", map[string]any{"delaySeconds": 5, "reason": "auto-continue tugas panjang", "prompt": contPrompt})
			return fmt.Sprintf("⏳ Tugasnya panjang — chunk %d kelar, gw jadwalin lanjutan OTOMATIS (nyambung sendiri sampe SELESAI).", next)
		}

		reqMap := map[string]any{"model": cfg.Model, "messages": msgs}
		if len(toolSpecs) > 0 {
			reqMap["tools"] = toolSpecs
			reqMap["parallel_tool_calls"] = false // 1 tool/iter → router aman (anti 400 multi tool_result)
		}
		reqJSON, _ := json.Marshal(reqMap)
		resp, err := callRouterRetry(reqJSON) // retry-backoff transient (transparan ke loop)
		if err != nil || resp == nil {
			return "Maaf, router LLM lagi ga stabil — udah gw coba ulang beberapa kali tapi belum nyambung. Coba lagi sebentar ya."
		}
		var o struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(resp.Body, &o) != nil || len(o.Choices) == 0 {
			return "(no response)"
		}
		m := o.Choices[0].Message
		if len(m.ToolCalls) == 0 {
			// GHOST-GUARD: narasi niat-aksi tanpa tool → paksa 1 putaran (bounded).
			if ghostNudges < maxGhostNudges && looksLikeGhostPromise(m.Content) {
				ghostNudges++
				c := m.Content
				if strings.TrimSpace(c) == "" {
					c = "(niat tanpa tool)"
				}
				msgs = append(msgs, map[string]any{"role": "assistant", "content": c})
				msgs = append(msgs, map[string]any{"role": "user", "content": ghostNudgeMsg})
				fmt.Fprintf(os.Stderr, "[%s] ghost-guard: nudge %d (narasi tanpa tool)\n", selfID(), ghostNudges)
				continue
			}
			return m.Content // jawaban final
		}
		// SERIALIZE: proses tool_call PERTAMA aja per iterasi (router aman).
		tc := m.ToolCalls[0]
		id := fmt.Sprintf("call_%d", iter)
		content := m.Content
		if strings.TrimSpace(content) == "" {
			content = "(memanggil tool)"
		}
		msgs = append(msgs, map[string]any{"role": "assistant", "content": content,
			"tool_calls": []any{map[string]any{"id": id, "type": "function",
				"function": map[string]any{"name": tc.Function.Name, "arguments": tc.Function.Arguments}}}})
		var targs map[string]any
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &targs)
		}
		fmt.Fprintf(os.Stderr, "[%s] tool_call: %s args=%s\n", selfID(), tc.Function.Name, truncStr(tc.Function.Arguments, 120))
		result := runTool(tc.Function.Name, targs)
		captureRecovery(tc.Function.Name, result, recovered) // recovery-instinct: error→sukses tool sama
		msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": id, "content": result})
		// #2C deferred-activate (seam, Rule 7): abis tool_lookup, ambil-ulang specs biar
		// tool yg baru di-discover MASUK array `tools` → model bisa manggilnya iterasi
		// berikut. HOST-gated (ToolSpecsHandler cuma nambah saat FLOWORK_DEFER_TOOLS on +
		// tool udah di-lookup) → no-op kalau defer off. Ini yg bikin all-tools/#2C jalan
		// di semua semut (bukan cuma mr-flow).
		if tc.Function.Name == "tool_lookup" {
			// Guard nil (lebih aman dari mr-flow): kalau re-fetch transient gagal/kosong,
			// JANGAN buang toolset yg ada (model masih bisa tool_lookup/tool_search).
			if ns := fetchToolSpecs(); len(ns) > 0 {
				toolSpecs = ns
			}
		}
		// FLAIL-GUARD: mantok = tool SAMA berulang tanpa progress (lolos ghost-guard +
		// recovery). Koreksi keras bounded → kalau tetep mantok lewat batas, eskalasi
		// JUJUR ke owner (bukan hard-stop, bukan ngarang udah-kelar).
		if corrective, nudge, escalate := flail.check(tc.Function.Name, tc.Function.Arguments); nudge {
			msgs = append(msgs, map[string]any{"role": "user", "content": corrective})
			fmt.Fprintf(os.Stderr, "[%s] flail-guard: nudge %d (tool=%s)\n", selfID(), flail.nudges, tc.Function.Name)
		} else if escalate {
			fmt.Fprintf(os.Stderr, "[%s] flail-guard: ESCALATE (tool=%s, mantok lewat batas koreksi)\n", selfID(), tc.Function.Name)
			return flailEscalation(tc.Function.Name)
		}
	}
	return "(tool loop limit — coba lagi/perjelas)"
}

func fetchSelfPrompt() string {
	resp, err := fetch("GET", selfPromp+selfID(), nil, nil, 2500)
	if err != nil || resp == nil || resp.Status >= 400 {
		return ""
	}
	var out struct {
		Rendered string `json:"rendered"` // self-prompt render (directive + DNA)
	}
	if json.Unmarshal(resp.Body, &out) != nil {
		return ""
	}
	return out.Rendered
}

func fetchToolSpecs() []json.RawMessage {
	resp, err := fetch("GET", specsURL+selfID(), nil, nil, 2500)
	if err != nil || resp == nil || resp.Status >= 400 {
		return nil
	}
	var out struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(resp.Body, &out) != nil {
		return nil
	}
	return out.Tools
}

func runTool(name string, args map[string]any) string {
	reqBody, _ := json.Marshal(map[string]any{"tool_name": name, "args": args, "caller": selfID() + "-loop"})
	resp, err := fetch("POST", toolRun+selfID(), map[string]string{"Content-Type": "application/json"}, reqBody, 30000)
	if err != nil || resp == nil {
		return `{"error":"tool dispatch gagal"}`
	}
	if resp.Status >= 400 {
		return fmt.Sprintf(`{"error":"tool http %d"}`, resp.Status)
	}
	out := string(resp.Body)
	if len(out) > 8*1024 {
		out = out[:8*1024] + " …[truncated]"
	}
	return out
}

// parseAutoCont — pesan LANJUTAN ("[LANJUTAN OTOMATIS #N] ...===TUGAS===<task>") →
// (task asli, N). Fresh (tanpa marker) → (s, 0). Counter ride di prompt (stateless).
func parseAutoCont(s string) (base string, count int) {
	const pfx = "[LANJUTAN OTOMATIS #"
	if !strings.HasPrefix(s, pfx) {
		return s, 0
	}
	end := strings.IndexByte(s, ']')
	if end < 0 || end <= len(pfx) {
		return s, 0
	}
	for _, r := range s[len(pfx):end] {
		if r >= '0' && r <= '9' {
			count = count*10 + int(r-'0')
		}
	}
	if k := strings.Index(s, autoContDelim); k >= 0 {
		return strings.TrimSpace(s[k+len(autoContDelim):]), count
	}
	return s, count
}

// truncStr — potong string buat log (TinyGo-safe).
func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
