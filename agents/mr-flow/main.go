// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Mr.Flow WASM agent (CRITICAL). Audit pass:
//   - Token + TELEGRAM_ALLOWED_CHATS validation (drop kalau invalid)
//   - chatID whitelist check (drop unauthorized + log decision skip_task)
//   - LLM failure detection via EXACT prefix match (anti false-positive
//     "router error:", "decode:", "llm:", "(no choices)", "")
//   - Length cap 3900 chars per Telegram message limit
//   - Anti-halu guards di callLLM (CURRENT_TIME_UTC inject, IDENTITY guard,
//     helpfulness rule, anti-invent tool whitelist)
//   - Skill auto-inject MAX 3 (maxActiveSkills), per-item 300 char (maxSkillCharsPerItem),
//     total persona 4000 char (maxPersonaTotalChars) — anti over-prompt
//   - Karma update best-effort (silent error OK)
//   - Self-prompt slot prepend (Section 35 phase 2)
//   - Log interactions + decisions per chat outcome
//
// mr-flow — Telegram AI agent untuk Flowork.
//
// === Layout standar (HARDCODED, lihat doc/standar_ai_agent.md) ===
//
//   /workspace/             ← workspace privat agent (mount kernel)
//   /workspace/state.db     ← SQLite per-agent
//   /shared/<id>/tools/     ← tools yang agent bikin sendiri
//   /shared/<id>/job/       ← output kerjaan
//   /shared/<id>/document/  ← markdown/notes/report
//   /shared/<id>/media/     ← audio/video/image
//   /shared/<id>/cache/     ← cache temporary
//   /shared/<id>/log/       ← log
//   /shared/_global/        ← bahan bareng lintas-agent
//
// === Sumber config (kernel inject) ===
//
//   FLOWORK_AGENT_CONFIG   — JSON utuh (prompt, router, schedule, skills)
//   TELEGRAM_BOT_TOKEN     — secrets.TELEGRAM_BOT_TOKEN dari popup
//   TELEGRAM_ALLOWED_CHATS — secrets.TELEGRAM_ALLOWED_CHATS dari popup
//   FLOWORK_AGENT_ID       — id agent (mr-flow)
//
// ABI (command pattern): kernel invoke binary dengan
//   os.Args[0] = "agent"
//   os.Args[1] = function name (boot | handle_message | send_message)
//   os.Args[2] = args JSON
// Output: JSON response ke stdout.

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

//go:wasmimport flowork host_net_fetch
func hostNetFetch(reqPtr, reqLen, outPtr, outMax uint32) uint32

//go:wasmimport flowork host_log_interaction
func hostLogInteraction(reqPtr, reqLen, outPtr, outMax uint32) uint32

//go:wasmimport flowork host_log_decision
func hostLogDecision(reqPtr, reqLen, outPtr, outMax uint32) uint32

//go:wasmimport flowork host_karma_update
func hostKarmaUpdate(reqPtr, reqLen, outPtr, outMax uint32) uint32

//go:wasmimport flowork host_time_now_ms
func hostTimeNowMs() uint64

//go:wasmimport flowork host_slash_dispatch
func hostSlashDispatch(reqPtr, reqLen, outPtr, outMax uint32) uint32

// === Path konstanta (HARDCODED standar Flowork) ===
const (
	WorkspacePrivate = "/workspace"            // mount per-agent (eksklusif)
	WorkspaceDB      = "/workspace/state.db"   // SQLite per-agent
	WorkspaceShared  = "/shared"               // mount shared workspace (root project)
)

const (
	defaultRouter  = "http://127.0.0.1:2402/v1/chat/completions"
	defaultModel   = "claude-haiku-4-5"
	defaultPersona = "Lo Mr.Flow — AI Agent di Flowork microkernel buat Mr.Dev. " +
		"Reply natural Bahasa Indonesia santai (bro/lo/gw OK), concise, no markdown headers. " +
		"Kalau gak yakin, bilang gak yakin. Hindari halu."
	pollTimeout  = 25 // seconds
	respBufBytes = 524288
)

// === Prompt Budget (per doc/standar_ai_agent.md section 11) ===
//
// Over-prompt = silent killer terutama buat local LLM (context window kecil).
// Setiap layer yang inject ke system prompt WAJIB respect budget di sini.
// On-demand fetch via tool call lebih baik daripada always-inject.
const (
	maxActiveSkills      = 3    // max skill auto-inject ke persona (sisanya via skill_search)
	maxSkillCharsPerItem = 300  // truncate instruction skill kalau terlalu panjang
	maxPersonaTotalChars = 4000 // hard cap persona total (~1000 token approx)
)

var outBuf [respBufBytes]byte

// ── Config dari kernel (sumber SQLite-backed) ──────────────────────────────

// Skill — entry dari config.skills[].
type Skill struct {
	ID           string `json:"id"`
	Trigger      string `json:"trigger"`
	Instructions string `json:"instructions"`
}

type agentConfig struct {
	Prompt string `json:"prompt"`
	Router struct {
		URL   string `json:"url"`
		Model string `json:"model"`
	} `json:"router"`
	Skills []Skill `json:"skills"`
}

// loadConfig parse FLOWORK_AGENT_CONFIG kalau ada, fallback ke default.
func loadConfig() agentConfig {
	cfg := agentConfig{}
	cfg.Router.URL = defaultRouter
	cfg.Router.Model = defaultModel
	cfg.Prompt = defaultPersona

	raw := strings.TrimSpace(os.Getenv("FLOWORK_AGENT_CONFIG"))
	if raw == "" {
		return cfg
	}
	var parsed agentConfig
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] FLOWORK_AGENT_CONFIG parse error: %v\n", err)
		return cfg
	}
	if parsed.Prompt != "" {
		cfg.Prompt = parsed.Prompt
	}
	if parsed.Router.URL != "" {
		cfg.Router.URL = parsed.Router.URL
	}
	if parsed.Router.Model != "" {
		cfg.Router.Model = parsed.Router.Model
	}
	cfg.Skills = parsed.Skills
	return cfg
}

// ── Entry ──────────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) < 2 {
		emit(map[string]string{"error": "missing function"})
		return
	}
	fn := os.Args[1]
	argsRaw := ""
	if len(os.Args) >= 3 {
		argsRaw = os.Args[2]
	}
	switch fn {
	case "boot":
		runDaemon()
	case "handle_message":
		doHandle(argsRaw)
	case "send_message":
		doSendAdmin(argsRaw)
	default:
		emit(map[string]string{"error": "unknown function: " + fn})
	}
}

// ── Daemon ─────────────────────────────────────────────────────────────────

func runDaemon() {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		fmt.Fprintln(os.Stderr, "[mr-flow] TELEGRAM_BOT_TOKEN belum di-set. Buka Setting → Credentials di popup, tambahin key 'TELEGRAM_BOT_TOKEN' = bot token dari @BotFather.")
		emit(map[string]string{"error": "TELEGRAM_BOT_TOKEN not set"})
		return
	}
	allowedRaw := strings.TrimSpace(os.Getenv("TELEGRAM_ALLOWED_CHATS"))
	if allowedRaw == "" {
		fmt.Fprintln(os.Stderr, "[mr-flow] TELEGRAM_ALLOWED_CHATS belum di-set. Buka Setting → Credentials, tambahin key 'TELEGRAM_ALLOWED_CHATS' = chat_id (pisah koma kalau lebih dari satu).")
		emit(map[string]string{"error": "TELEGRAM_ALLOWED_CHATS not set"})
		return
	}
	allowed := map[int64]bool{}
	for _, s := range strings.Split(allowedRaw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			allowed[n] = true
		}
	}
	if len(allowed) == 0 {
		emit(map[string]string{"error": "no valid chat_id in TELEGRAM_ALLOWED_CHATS"})
		return
	}

	cfg := loadConfig()
	fmt.Fprintf(os.Stderr, "[mr-flow] daemon ready: %d allowed chats, router=%s model=%s, skills=%d\n",
		len(allowed), cfg.Router.URL, cfg.Router.Model, len(cfg.Skills))

	var offset int64
	for {
		updates, err := getUpdates(token, offset, pollTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mr-flow] getUpdates err: %v\n", err)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			chatID := u.Message.Chat.ID
			if !allowed[chatID] {
				fmt.Fprintf(os.Stderr, "[mr-flow] drop unauthorized chat=%d\n", chatID)
				// Section 3: log decision skip_task (drop chat unauthorized).
				logDecision("skip_task",
					"chat_id ngga ada di TELEGRAM_ALLOWED_CHATS — drop",
					"success",
					map[string]any{
						"chat_id":    chatID,
						"message_id": u.Message.MessageID,
					},
					0)
				continue
			}
			fmt.Fprintf(os.Stderr, "[mr-flow] received chat=%d text=%q\n", chatID, truncStr(u.Message.Text, 80))
			logInteraction("telegram", "in", strconv.FormatInt(chatID, 10), u.Message.Text, map[string]any{
				"message_id": u.Message.MessageID,
				"update_id":  u.UpdateID,
			})
			// Section 17: leading "/" → dispatch slash command, skip LLM.
			// Caller format: telegram:<chat_id>.
			if strings.HasPrefix(strings.TrimSpace(u.Message.Text), "/") {
				slashCaller := "telegram:" + strconv.FormatInt(chatID, 10)
				slashReply, _ := dispatchSlash(u.Message.Text, slashCaller)
				if slashReply == "" {
					slashReply = "(slash dispatcher returned empty result)"
				}
				if len(slashReply) > 3900 {
					slashReply = slashReply[:3900] + "\n…(truncated)"
				}
				if err := sendMessage(token, chatID, slashReply); err != nil {
					fmt.Fprintf(os.Stderr, "[mr-flow] sendMessage err (slash): %v\n", err)
				} else {
					logInteraction("telegram", "out", strconv.FormatInt(chatID, 10), slashReply, map[string]any{
						"source":           "slash",
						"reply_to_message": u.Message.MessageID,
					})
				}
				continue
			}
			sendTyping(token, chatID)
			// Section 5: time the LLM call for avg_response_ms moving avg.
			// TinyGo wasi target's time.Since() returns wrong precision —
			// pakai host capability host_time_now_ms (wall-clock ms reliable).
			t0Ms := hostTimeNowMs()
			// Ambil konteks percakapan (sudah termasuk pesan ini yang barusan
			// di-log di atas) supaya Mr.Flow inget obrolan sebelumnya.
			hist := fetchHistory(strconv.FormatInt(chatID, 10))
			reply := callLLM(cfg, u.Message.Text, hist)
			elapsedMs := float64(hostTimeNowMs() - t0Ms)
			// Detect LLM failure via exact known error prefixes from callLLM
			// (sumber: callLLM returns: "router error:", "decode:", "llm:",
			// "(no choices)", or "" for empty). JANGAN pakai "(LLM " — itu
			// ngga pernah keluar dan bisa false-positive reply LLM yang
			// kebetulan mulai "(LLM..." (audit Section 3 finding).
			origReply := reply
			llmFailed := reply == "" ||
				strings.HasPrefix(reply, "router error:") ||
				strings.HasPrefix(reply, "decode:") ||
				strings.HasPrefix(reply, "llm:") ||
				reply == "(no choices)"
			// Section 5: karma update — counter (success/fail) + moving avg
			// response time. Best-effort silent error.
			fmt.Fprintf(os.Stderr, "[mr-flow] llm took %vms (llmFailed=%v)\n", elapsedMs, llmFailed)
			if llmFailed {
				logKarma("increment", "fail_count", 1)
			} else {
				logKarma("increment", "success_count", 1)
				logKarma("average", "avg_response_ms", elapsedMs)
			}
			if reply == "" {
				reply = "(LLM returned no text)"
			}
			fmt.Fprintf(os.Stderr, "[mr-flow] reply len=%d preview=%q\n", len(reply), truncStr(reply, 80))
			if len(reply) > 3900 {
				reply = reply[:3900] + "\n…(truncated)"
			}
			// Section 3: log decision model_choice (sukses) atau escalate (fail).
			// Log `reply_head` capture origReply (sebelum overwrite ke fallback
			// "(LLM returned no text)") supaya debug actionable.
			if llmFailed {
				logDecision("escalate",
					"LLM call gagal / kosong — reply fallback ke user. Cek router :2402 + provider quota.",
					"fail",
					map[string]any{
						"model":      cfg.Router.Model,
						"router":     cfg.Router.URL,
						"reply_head": truncStr(origReply, 120),
					},
					0)
			} else {
				logDecision("model_choice",
					"Dispatch ke router primary model (sukses).",
					"success",
					map[string]any{
						"model":      cfg.Router.Model,
						"chat_id":    chatID,
						"reply_len":  len(origReply),
						"reply_head": truncStr(origReply, 120),
					},
					0)
			}
			if err := sendMessage(token, chatID, reply); err != nil {
				fmt.Fprintf(os.Stderr, "[mr-flow] sendMessage err: %v\n", err)
			} else {
				logInteraction("telegram", "out", strconv.FormatInt(chatID, 10), reply, map[string]any{
					"model":            cfg.Router.Model,
					"reply_to_message": u.Message.MessageID,
				})
			}
		}
	}
}

// ── Telegram primitives ────────────────────────────────────────────────────

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type Chat struct {
	ID int64 `json:"id"`
}

func getUpdates(token string, offset int64, timeoutSec int) ([]Update, error) {
	u := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?timeout=%d&allowed_updates=%%5B%%22message%%22%%5D",
		token, timeoutSec,
	)
	if offset > 0 {
		u += fmt.Sprintf("&offset=%d", offset)
	}
	resp, err := fetch("GET", u, nil, nil, (timeoutSec+5)*1000)
	if err != nil {
		return nil, err
	}
	if resp.Status >= 400 {
		return nil, fmt.Errorf("telegram %d: %s", resp.Status, truncStr(string(resp.Body), 160))
	}
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		return nil, fmt.Errorf("telegram envelope ok=false: %s", truncStr(string(resp.Body), 160))
	}
	var updates []Update
	_ = json.Unmarshal(env.Result, &updates)
	return updates, nil
}

func sendMessage(token string, chatID int64, text string) error {
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := fetch("POST", u, map[string]string{"Content-Type": "application/json"}, body, 15_000)
	if err != nil {
		return err
	}
	if resp.Status >= 400 {
		return fmt.Errorf("telegram %d: %s", resp.Status, truncStr(string(resp.Body), 160))
	}
	return nil
}

func sendTyping(token string, chatID int64) {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "action": "typing"})
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", token)
	_, _ = fetch("POST", u, map[string]string{"Content-Type": "application/json"}, body, 5_000)
}

// ── LLM router ─────────────────────────────────────────────────────────────

// callLLM kirim user text + persona + skills metadata ke router.
//
// === Prompt Budget enforcement ===
// Auto-inject MAX maxActiveSkills (= 3) skill ke persona. Sisanya available
// tapi warga butuh panggil tool `skill_search` untuk fetch. Mencegah
// over-prompt yang bikin LLM halu (terutama local LLM context kecil).
//
// Per skill: max maxSkillCharsPerItem chars (truncate instruction). Per
// total persona: max maxPersonaTotalChars (hard cap).
// nowISO format current time dari host_time_now_ms ke "YYYY-MM-DD HH:MM UTC".
// Inject ke persona supaya LLM punya ground truth tanggal (anti-halu cutoff).
func nowISO() string {
	ms := hostTimeNowMs()
	return time.Unix(int64(ms/1000), 0).UTC().Format("2006-01-02 15:04 UTC")
}

// chatTurn — satu giliran percakapan untuk konteks LLM.
type chatTurn struct {
	Role    string // "user" | "assistant"
	Content string
}

const (
	maxHistoryMsgs        = 16   // max giliran percakapan di-inject (≈8 tukar-balik)
	maxHistoryCharsPerMsg = 1200 // cap per pesan history (anti over-prompt)
)

// fetchHistory — ambil riwayat percakapan chat ini dari API interactions
// agent sendiri (pola sama fetchSelfPrompt). Persistent (baca dari state.db),
// jadi memory survive restart. Return turn KRONOLOGIS (lama→baru), SUDAH
// termasuk pesan terakhir user (yang barusan di-log sebelum callLLM).
// Empty kalau API unreachable → caller fallback ke single user message.
func fetchHistory(actor string) []chatTurn {
	if actor == "" {
		return nil
	}
	url := "http://127.0.0.1:1987/api/agents/interactions?id=mr-flow&limit=40"
	resp, err := fetch("GET", url, nil, nil, 2500)
	if err != nil || resp == nil || resp.Status >= 400 {
		return nil
	}
	var out struct {
		Items []struct {
			Direction string `json:"direction"`
			Actor     string `json:"actor"`
			Content   string `json:"content"`
		} `json:"items"`
	}
	if json.Unmarshal(resp.Body, &out) != nil {
		return nil
	}
	// items newest-first → kumpulin yang match actor ini, stop di cap, reverse.
	picked := make([]chatTurn, 0, maxHistoryMsgs)
	for _, it := range out.Items {
		if it.Actor != actor || it.Content == "" {
			continue
		}
		role := "user"
		if it.Direction == "out" {
			role = "assistant"
		}
		c := it.Content
		if len(c) > maxHistoryCharsPerMsg {
			c = c[:maxHistoryCharsPerMsg] + "…"
		}
		picked = append(picked, chatTurn{Role: role, Content: c})
		if len(picked) >= maxHistoryMsgs {
			break
		}
	}
	for i, j := 0, len(picked)-1; i < j; i, j = i+1, j-1 {
		picked[i], picked[j] = picked[j], picked[i]
	}
	return picked
}

func callLLM(cfg agentConfig, userText string, history []chatTurn) string {
	// Anti-halu identity + time guard. Prepend ke persona setiap call —
	// LLM (claude-haiku-4-5 via flow_router) kalo gak di-reminded suka
	// claim "training cutoff May 2024" dan halu tanggal hari ini. Mr.Flow
	// itu wrapper WASM yang call router, bukan model base; identity guard
	// disable claim training cutoff sendiri. Time guard kasih ground truth.
	guard := fmt.Sprintf(
		"[CURRENT_TIME_UTC: %s]\n"+
			"[IDENTITY: Lo Mr.Flow — WASM agent di Flowork microkernel buat Mr.Dev. "+
			"Lo BUKAN Claude/GPT/model base. Lo wrapper yang dispatch ke "+
			"flow_router. Jangan claim \"training cutoff\" — lo ngga punya "+
			"training history sendiri. Kalo ditanya tanggal, pakai "+
			"CURRENT_TIME_UTC di atas.]\n"+
			"[HELPFULNESS RULE: Mr.Dev minta info real-time (harga, berita, "+
			"status live)? BANTU LANGSUNG — jangan defensive nyuruh dia 'cek "+
			"sendiri'. Step-nya: (1) kasih konteks dari knowledge umum lo "+
			"dengan caveat 'data ini stale, terakhir update knowledge umum'; "+
			"(2) sebut 1-2 source live yang relevan singkat (CoinGecko, "+
			"CoinMarketCap utk harga crypto; Reuters/CNBC utk news); (3) "+
			"tawarin bantu lebih lanjut. Contoh BURUK: 'gw gak punya real-time, "+
			"lo cek sendiri di X'. Contoh BAIK: 'BTC sekitar $X di knowledge "+
			"terakhir gw (stale). Live cek CoinGecko/Binance. Mau gw bantu "+
			"breakdown trend atau historical context?']\n"+
			"[ANTI-INVENT TOOL: Kalo mau suggest tool, sebut yg real ada di "+
			"capability lo (net:fetch:telegram, net:fetch:router, state:read/"+
			"write, time:read, fs:read/write, exec:git, rpc:router:skill). "+
			"Jangan invent nama tool kayak 'web_search' atau 'brain_search' "+
			"kalo gak ada di list itu.]\n"+
			"[NO FAKE EXECUTION: Dari chat ini lo CUMA bisa balas teks — lo "+
			"GAK bisa jalanin tool async / fetch live / scan real-time terus "+
			"kasih hasilnya di pesan terpisah. JANGAN pura-pura lagi "+
			"'scanning…', 'querying…', 'fetching…', atau janji 'tunggu output "+
			"gw'. Itu HALU dan bikin Mr.Dev kesel. Kalo ga bisa real-time, "+
			"bilang jujur + kasih cara/sumber manual. Lo PUNYA konteks "+
			"percakapan sebelumnya di messages ini — pakai, jangan tanya ulang "+
			"hal yang udah dibahas.]\n\n",
		nowISO(),
	)
	persona := guard + cfg.Prompt
	if len(cfg.Skills) > 0 {
		// Auto-inject hanya N skill pertama (asumsi ordered by importance).
		active := cfg.Skills
		if len(active) > maxActiveSkills {
			active = active[:maxActiveSkills]
		}
		var lines []string
		for _, s := range active {
			instr := s.Instructions
			if len(instr) > maxSkillCharsPerItem {
				instr = instr[:maxSkillCharsPerItem] + "…"
			}
			lines = append(lines, fmt.Sprintf("- %s (trigger=%q): %s", s.ID, s.Trigger, instr))
		}
		if extra := len(cfg.Skills) - len(active); extra > 0 {
			lines = append(lines, fmt.Sprintf("…+%d skill lain (panggil `skill_search` kalau perlu)", extra))
		}
		persona += "\n\nSkill aktif:\n" + strings.Join(lines, "\n")
	}
	// Section 35 phase 2: prepend self_prompt slots (system/persona/
	// guideline/task) dari /api/agents/self-prompt/render. Best-effort —
	// kalau endpoint unreachable, skip + lanjut. Aborted via short timeout.
	if injected := fetchSelfPrompt(); injected != "" {
		persona = injected + "\n\n" + persona
	}
	// Hard cap persona total — last-ditch defense.
	if len(persona) > maxPersonaTotalChars {
		persona = persona[:maxPersonaTotalChars] + "\n…[truncated to respect prompt budget]"
	}

	// Bangun messages: system + history percakapan + (fallback user kalau
	// history kosong). history SUDAH termasuk pesan user terakhir, jadi kalau
	// ada history kita ngga append userText lagi (anti dobel).
	msgs := []map[string]string{{"role": "system", "content": persona}}
	if len(history) > 0 {
		for _, t := range history {
			msgs = append(msgs, map[string]string{"role": t.Role, "content": t.Content})
		}
	} else {
		msgs = append(msgs, map[string]string{"role": "user", "content": userText})
	}
	body, _ := json.Marshal(map[string]any{
		"model":    cfg.Router.Model,
		"messages": msgs,
	})

	resp, err := fetch("POST", cfg.Router.URL,
		map[string]string{"Content-Type": "application/json"}, body, 90_000)
	if err != nil {
		return "router error: " + err.Error()
	}
	if resp.Status >= 400 {
		return fmt.Sprintf("router %d: %s", resp.Status, truncStr(string(resp.Body), 200))
	}
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(resp.Body, &openaiResp); err != nil {
		return "decode: " + err.Error()
	}
	if openaiResp.Error != nil {
		errBytes, _ := json.Marshal(openaiResp.Error)
		return "llm: " + string(errBytes)
	}
	if len(openaiResp.Choices) == 0 {
		return "(no choices)"
	}
	return openaiResp.Choices[0].Message.Content
}

// ── Direct RPC handlers ────────────────────────────────────────────────────

func doHandle(argsRaw string) {
	var in struct {
		Text string `json:"text"`
		User string `json:"user"`
	}
	if err := json.Unmarshal([]byte(argsRaw), &in); err != nil {
		emit(map[string]any{"reply": "parse: " + err.Error()})
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		emit(map[string]any{"reply": "kosong bro, kirim pesan dulu"})
		return
	}
	// Section 17 parity: leading "/" → dispatch slash command, skip LLM.
	// Same path as runDaemon (Telegram path). Tanpa branch ini, RPC entry
	// (chat-debug script + future webhook) bypass slash dispatcher.
	if strings.HasPrefix(in.Text, "/") {
		caller := strings.TrimSpace(in.User)
		if caller == "" {
			caller = "rpc"
		}
		if reply, ok := dispatchSlash(in.Text, caller); ok {
			emit(map[string]any{"reply": reply})
			return
		}
		// Slash unknown atau dispatch error → fallback LLM agar user dapet
		// respons (better UX dari error mentah).
	}
	// Jalur RPC mirror Telegram: log in → fetch history → callLLM → log out.
	// Bikin chat-debug (flowork-cli) punya memory percakapan yang sama dengan
	// Telegram, dan bisa di-test (doktrin: test lewat jalur yang sama).
	actor := strings.TrimSpace(in.User)
	if actor == "" {
		actor = "rpc"
	}
	logInteraction("rpc", "in", actor, in.Text, map[string]any{})
	hist := fetchHistory(actor)
	reply := callLLM(loadConfig(), in.Text, hist)
	logInteraction("rpc", "out", actor, reply, map[string]any{"model": loadConfig().Router.Model})
	emit(map[string]any{"reply": reply})
}

func doSendAdmin(argsRaw string) {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if token == "" {
		emit(map[string]string{"error": "TELEGRAM_BOT_TOKEN not set"})
		return
	}
	var in struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal([]byte(argsRaw), &in); err != nil {
		emit(map[string]string{"error": "parse: " + err.Error()})
		return
	}
	if in.ChatID == 0 || in.Text == "" {
		emit(map[string]string{"error": "chat_id + text required"})
		return
	}
	if err := sendMessage(token, in.ChatID, in.Text); err != nil {
		emit(map[string]string{"error": err.Error()})
		return
	}
	emit(map[string]any{"ok": true})
}

// ── HTTP wrapper via host capability ───────────────────────────────────────

type httpResp struct {
	Status int
	Body   []byte
}

func fetch(method, url string, headers map[string]string, body []byte, timeoutMS int) (*httpResp, error) {
	req := map[string]any{
		"method":         method,
		"url":            url,
		"timeout_ms":     timeoutMS,
		"max_resp_bytes": 4 << 20,
	}
	if len(headers) > 0 {
		req["headers"] = headers
	}
	if len(body) > 0 {
		req["body_base64"] = base64.StdEncoding.EncodeToString(body)
	}
	reqJSON, _ := json.Marshal(req)

	written := hostNetFetch(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(outBuf[:]), uint32(len(outBuf)),
	)
	if written == 0 {
		return nil, fmt.Errorf("host_net_fetch returned 0 bytes")
	}
	var hostResp struct {
		Status  int    `json:"status"`
		BodyB64 string `json:"body_base64"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(outBuf[:written], &hostResp); err != nil {
		return nil, fmt.Errorf("decode host response: %w", err)
	}
	if hostResp.Error != "" {
		return nil, fmt.Errorf("host: %s", hostResp.Error)
	}
	bodyBytes, _ := base64.StdEncoding.DecodeString(hostResp.BodyB64)
	return &httpResp{Status: hostResp.Status, Body: bodyBytes}, nil
}

func emit(v any) {
	body, _ := json.Marshal(v)
	fmt.Println(string(body))
}

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// logInteraction — append row ke tabel `interactions` di state.db agent
// ini lewat host capability. Best-effort (silent on error supaya daemon
// loop ngga crash kalau DB lock sementara). channel = 'telegram',
// direction = 'in' | 'out'.
//
// ⚠️ JANGAN baca log ini dari WASM ke system prompt. Akses cuma via
// HTTP endpoint atau tool call (lihat standar_ai_agent.md section 11).
//
// logBuf 4KB: host cap error message ke 400 char, tapi sukses response
// `{"ok":true}` 12B saja. 4KB jaga margin kalau metadata error JSON
// bengkak (path full, stacktrace mini).
var logBuf [4096]byte

// logDecision — append row ke tabel `decisions` di state.db agent ini
// lewat host capability `host_log_decision`. Best-effort (silent on error
// supaya daemon loop ngga crash kalau DB lock).
//
// decisionType: 'model_choice' | 'skip_task' | 'escalate' | 'tool_pick' | dst
// outcome: 'success' | 'fail' | 'pending' (kosong → 'pending' di host)
//
// ⚠️ JANGAN baca log ini ke system prompt. Akses cuma via HTTP endpoint
// atau tool call (lihat standar_ai_agent.md section 11).
var decisionBuf [4096]byte

// dispatchSlash — wrap host_slash_dispatch. Return result text + ok flag.
// Caller branch ke LLM kalau ok=false (parse fail atau command not found).
//
// ⚠️ Buffer 16KB cukup untuk slash result yang text-only (cap 8KB di host).
var slashBuf [16384]byte

func dispatchSlash(text, caller string) (resultText string, ok bool) {
	req := map[string]any{"text": text}
	if caller != "" {
		req["caller"] = caller
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] slash marshal: %v\n", err)
		return "", false
	}
	written := hostSlashDispatch(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(slashBuf[:]), uint32(len(slashBuf)),
	)
	if written == 0 {
		fmt.Fprintf(os.Stderr, "[mr-flow] slash returned 0 bytes\n")
		return "", false
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Text    string `json:"text"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(slashBuf[:written], &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] slash decode: %v\n", err)
		return "", false
	}
	if !resp.OK {
		// Error path masih return string supaya caller bisa kasih ke user
		// (mis. "command not found: /xyz"). Tapi ok=false → caller mungkin
		// pilih fallback ke LLM atau just emit error.
		return resp.Error, false
	}
	return resp.Text, true
}

// logKarma — wrapper helper untuk hostKarmaUpdate. op = 'increment' atau
// 'average'. Best-effort silent error supaya daemon ngga crash kalau DB lock.
//
// Audit fix C3: pakai struct typed (bukan map[string]any) supaya konsisten
// dengan Section 1/3 pattern + JSON key order deterministic di TinyGo.
//
// ⚠️ JANGAN inject karma value ke system prompt (anti over-prompt). Akses
// HTTP endpoint only.
var karmaBuf [1024]byte

type karmaReq struct {
	Op    string  `json:"op"`
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

func logKarma(op, key string, value float64) {
	req := karmaReq{Op: op, Key: key, Value: value}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] karma marshal: %v\n", err)
		return
	}
	written := hostKarmaUpdate(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(karmaBuf[:]), uint32(len(karmaBuf)),
	)
	if written == 0 {
		fmt.Fprintf(os.Stderr, "[mr-flow] host_karma_update returned 0 bytes\n")
		return
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(karmaBuf[:written], &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] karma decode: %v\n", err)
		return
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "[mr-flow] karma err: %s\n", resp.Error)
	}
}

func logDecision(decisionType, rationale, outcome string, inputs map[string]any, refInteractionID int64) {
	req := map[string]any{
		"decision_type": decisionType,
		"rationale":     rationale,
	}
	if outcome != "" {
		req["outcome"] = outcome
	}
	if len(inputs) > 0 {
		req["inputs"] = inputs
	}
	if refInteractionID > 0 {
		req["ref_interaction_id"] = refInteractionID
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] decision marshal: %v\n", err)
		return
	}
	written := hostLogDecision(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(decisionBuf[:]), uint32(len(decisionBuf)),
	)
	if written == 0 {
		fmt.Fprintf(os.Stderr, "[mr-flow] host_log_decision returned 0 bytes\n")
		return
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(decisionBuf[:written], &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] decision decode: %v\n", err)
		return
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "[mr-flow] decision err: %s\n", resp.Error)
	}
}

func logInteraction(channel, direction, actor, content string, metadata map[string]any) {
	req := map[string]any{
		"channel":   channel,
		"direction": direction,
		"actor":     actor,
		"content":   content,
	}
	if len(metadata) > 0 {
		req["metadata"] = metadata
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] log marshal: %v\n", err)
		return
	}
	written := hostLogInteraction(
		bytesPtr(reqJSON), uint32(len(reqJSON)),
		bytesPtr(logBuf[:]), uint32(len(logBuf)),
	)
	if written == 0 {
		fmt.Fprintf(os.Stderr, "[mr-flow] host_log_interaction returned 0 bytes\n")
		return
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(logBuf[:written], &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[mr-flow] log decode: %v\n", err)
		return
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "[mr-flow] log err: %s\n", resp.Error)
	}
}

// fetchSelfPrompt — Section 35 phase 2: GET /api/agents/self-prompt/render?
// id=mr-flow. Best-effort: short timeout, swallow errors. Inject result
// sebagai prepended system prompt.
func fetchSelfPrompt() string {
	url := "http://127.0.0.1:1987/api/agents/self-prompt/render?id=mr-flow"
	resp, err := fetch("GET", url, nil, nil, 2000)
	if err != nil || resp == nil || resp.Status >= 400 {
		return ""
	}
	var out struct {
		Rendered string `json:"rendered"`
	}
	if jerr := json.Unmarshal(resp.Body, &out); jerr != nil {
		return ""
	}
	return strings.TrimSpace(out.Rendered)
}
