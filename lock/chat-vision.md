# 👁️ CHAT VISION — multimodal paste (Ctrl+V screenshot → LLM vision)

Status: LIVE (verified E2E 2026-07-02: gemini via Antigravity ✓ + claude via Anthropic ✓,
history multi-turn ✓, GUI render ✓). Feature files NON-FROZEN (deletable).

## Cara kerja (alur)
1. **GUI** `agent/web/js/chatui.js` — paste gambar di textarea Chat tab → chip preview
   (`.cu-attach`) → send `POST /api/chat/send {session_id, text, images:[dataURL]}`.
   Thumbnail dirender di bubble user + history. Maks 4 gambar/pesan.
2. **Agent** `agent/chat_sessions.go` (soft-lock) — terima `images`, validasi
   (`validChatImages`, data:image/*;base64, ≤8MB/gambar, ≤20MB total, body cap 16MB),
   persist via `AddChatMessageImages` → kolom `chat_message.images` (JSON array,
   ADDITIVE `ALTER TABLE ADD COLUMN`, `agent/internal/floworkdb/chatdb.go`).
3. **Architect brain** `agent/architect_chat.go` — turn user ber-gambar dibungkus
   `visionContent()` (`agent/chat_vision.go`) jadi **content-block JSON string**
   (konvensi sama dgn `preprocess_content.go`):
   `[{"type":"text","text":...},{"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}]`
4. **Router** decode string itu jadi format provider lewat 2 SEAM (Pola B):
   - `internal/executors/antigravity.go` (FROZEN) → `AntigravityPartsHook` — diisi
     sibling `antigravity_vision_ext.go` → parts Gemini `{"inlineData":{mimeType,data}}`.
   - `internal/router/tools.go` (FROZEN) → `anthropicUserContentHook` — diisi sibling
     `vision_anthropic_ext.go` → block Anthropic `{"type":"image","source":{base64,...}}`.
   - Parser bersama: `internal/visionblocks/` (Parse ketat: semua entri harus dikenal,
     wajib ada ≥1 gambar data-URL; selain itu → bukan block → teks apa adanya).

## Guard konteks (agent/llm_context_safe.go)
- `msgContentLen` → `chatContentEstLen`: block vision dihitung teks + ~6400 char/gambar
  (≈1600 token), BUKAN panjang base64 — tanpa ini compactor motong base64 (gambar korup).
- `compactMessages` SKIP truncate content block-vision (`isVisionBlockContent`).
- `ctxBudgetTokens`: + case gemini → 180000.

## Switch
- Kill-switch router: `FLOWORK_VISION=0` → hook ga dipasang (balik text-only).
- Hapus sibling ext (`antigravity_vision_ext.go`, `vision_anthropic_ext.go`,
  `internal/visionblocks/`, `agent/chat_vision.go` + pemakainya) → seam nil → aman
  (delete-test router PASS 2026-07-02).

## FIX 2026-07-02 (Telegram vision + jalur no-tools)
Bug: foto via Telegram → "error status 400" / model halu (token bengkak 23k). DUA akar:
1. **telegram_media.go** (`visionDescribe`) kirim `content` sbg ARRAY native → router
   `OpenAIMessage.Content` itu `string` → gagal unmarshal → HTTP 400. Fix: kirim
   content-block sbg JSON STRING (konvensi visionblocks, sama kayak GUI chat_vision.go).
2. **Jalur anthropic NO-TOOLS** (`forwardAnthropic`/`streamAnthropic`) pakai
   `AnthropicMessage.Content string` → ga bisa bikin image block → vision cuma jalan
   kalau kebetulan ada tool (GUI architect). Telegram no-tools → base64 nyasar jadi TEKS
   → model halu. Fix: gate frozen nambah `|| hasVisionContent(req)` (deteksi di
   `vision_route_ext.go` non-frozen) → request vision di-route ke jalur with-tools yg
   pasang image block. Verified: "KUCING ORANYE 77" kebaca bener, token 23474→1303.
Freeze: telegram_media.go (mr-flow wasm) + dispatcher.go + dispatcher_stream.go re-hash + re-freeze.

## AI STUDIO — chat langsung ke mr-flow (2026-07-02, verified live)
Owner minta: bisa ngobrol sama mr-flow LEWAT AI Studio (Chat tab), kirim doc+image,
+ indikator "lagi mikir" biar berasa hidup.
- **Dropdown target** (`chatui.js`): + opsi "🤖 Mr.Flow" (`agent:mr-flow`) di samping
  Architect + tiap group. barValues → mode `agent`. chatdb `CreateChatSession` izinin mode "agent".
- **Backend** (`chat_sessions.go`): mode `agent` → `InvokeAgentMessage(targetID, ...)` (agent
  pegang memori sendiri → kirim pesan TERAKHIR aja, bukan replay). Verified: mr-flow jawab via AI Studio.
- **Image ke mr-flow**: agent intake TEXT-ONLY → `describeImagesForAgent` vision-describe dulu
  via router (pola Telegram) → tempel deskripsi ke teks → mr-flow "liat". Verified: KUCING ORANYE 77 kebaca.
- **Doc**: tombol 📎 (`chatui.js`) baca file teks client-side → tempel isi ke pesan (universal, semua target).
- **Indikator "lagi mikir"**: pending bubble = nama target + typing dots + timer detik live
  (`cu-think`/`cu-elapsed`). Verified via screenshot render.
- File (semua soft-lock, editable): `chatui.js` (embedded → rebuild binari) · `chat_sessions.go` ·
  `internal/floworkdb/chatdb.go`.

## Batas yang disengaja (bukan bug)
- Mode GROUP text-only: gambar ditandai teks `[📷 user melampirkan gambar]`
  (`buildGroupTranscript`) — vision penuh = jalur architect.
- Provider openai-compat/local (llama) ga di-decode → block string keliatan sbg teks
  JSON. Fallback-of-fallback, model lokal ga vision. Kalau nanti ada provider OpenAI
  vision: tambah seam serupa di marshal openai (pola sama, tiru 2 seam di atas).
- mr-flow WASM chat (/api/chat) bukan jalur ini (beda pipeline).

## File (status)
| File | Status |
|---|---|
| `router/internal/executors/antigravity.go` | FROZEN (hash updated 2026-07-02, seam nambah) |
| `router/internal/router/tools.go` | FROZEN (hash updated 2026-07-02, seam nambah) |
| `router/internal/executors/antigravity_vision_ext.go` | non-frozen, deletable |
| `router/internal/router/vision_anthropic_ext.go` | non-frozen, deletable |
| `router/internal/visionblocks/*` | non-frozen, deletable (+unit test) |
| `agent/chat_vision.go` | non-frozen, deletable |
| `agent/chat_sessions.go` / `architect_chat.go` / `llm_context_safe.go` / `internal/floworkdb/chatdb.go` | soft-lock (edit seizin roadmap owner 2026-07-02) |
| `agent/web/js/chatui.js` | soft-lock (embedded — ubah = rebuild binari) |
