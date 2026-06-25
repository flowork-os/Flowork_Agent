# MR-FLOW — Agent Owner (Jantung Flowork): Telegram I/O, Media, Format, Routing & Switch

> Dokumen referensi (white-label). mr-flow = agent owner, orchestrator utama, yang ngobrol sama
> owner di Telegram. Dok ini: arsitektur, I/O Telegram (format + baca dokumen/foto/voice), routing,
> switch, build, cabang, freeze. Owner: Aola Sahidin (Mr.Dev).
> Repo: https://github.com/flowork-os/Flowork-OS. Update: 2026-06-23.
> ⚠️ KE-TRACK repo → NOL data personal owner.

---

## ⛔ WAJIB BACA DULU

`agents/mr-flow/main.go` = **FROZEN brain-core** (chattr +i + hash KERNEL_FREEZE). **JANGAN buka.**
Filtur Telegram baru (format, tipe media, vision, dst) → **CABANG NON-frozen `agents/mr-flow/telegram_media.go`**
(+ switch env). main.go cuma manggil fungsi cabang. Routing/persona → §5. Arsitektur otak → `lock/brain.md`.

---

## 0. APA INI

mr-flow = **agent owner** (tier primary), jalan sbg **WASM** (wasip1, **standard Go** — bukan tinygo →
full stdlib). Tugas: long-poll Telegram → proses (LLM via router :2402 / route ke squad) → balas.
Persona di kv `prompt` (Settings GUI). Model per-agent kv `router_model` (default `flowork-brain`).

---

## 1. ARSITEKTUR I/O

```
Owner ⇄ Telegram Bot ⇄ mr-flow (WASM)
   getUpdates (long-poll) → Message{text | document | photo | voice | caption}
       │ media? → enrichMedia() [telegram_media.go] → teks
       ▼
   proses: slash? deterministic-route? → LLM (router) / task_run squad
       ▼
   sendMessage() → formatTelegram() [telegram_media.go] → Telegram HTML (rapi)
```

---

## 2. BACA MEDIA (dokumen/foto/voice) — `telegram_media.go`

Dulu mr-flow **cuma handle text** (`Text==""` → drop). 2026-06-23: media di-baca. Loop manggil
`enrichMedia(msg, token)` → ubah media jadi teks yg diproses LLM. Semua **GRACEFUL** (gagal = tetap
balas acknowledge, ga crash). Switch `FLOWORK_TG_MEDIA` (default ON; "off" = text-only).

| Tipe | Cara | Dependensi |
|---|---|---|
| **Dokumen** | getFile→download→`mediaDocument`: teks (txt/md/code/json/csv) dibaca isinya (cap 12k char); binary (pdf/docx) → note minta kirim teks | — |
| **Voice/Audio** | download→`sttTranscribe`: multipart POST `router /v1/audio/transcriptions` → transkrip | **STT provider AKTIF** (Settings → Media Providers: deepgram/assemblyai/gemini/openai). Ga ada → graceful note |
| **Foto** | download→base64→`visionDescribe`: POST chat endpoint `image_url` → deskripsi ("yang gw LIHAT…") | model **vision-capable** (Claude) + router pass image content. Ga support → acknowledge + caption |

Catatan: download via `fetch` (host bridge, base64 round-trip, **cap 4MB**). File > 4MB ke-truncate.
Foto pakai resolusi terbesar (`Photo[last]`).

---

## 3. FORMAT PESAN (rapi di Telegram) — `telegram_media.go`

**Akar:** LLM output markdown (`**bold**`, `` `code` ``, `# header`) tapi sendMessage dulu **tanpa
parse_mode** → muncul mentah = "ngak rapi". **Fix:** `formatTelegram()` convert markdown → **Telegram
HTML** (`<b>`/`<i>`/`<code>`/`<pre>`/`<a>`), sendMessage kirim dgn `parse_mode=HTML`. Code block/inline
di-"parkir" dulu biar isinya literal. **FALLBACK:** kalau HTML ditolak Telegram (400) → kirim ulang
POLOS (`stripMarkdown`) — pesan ga pernah ilang. Switch `FLOWORK_TG_FORMAT`: `html` (default) | `plain`/`off`.

---

## 4. SWITCH (env, NON-frozen — jalan evolusi)

| Switch | Default | Guna |
|---|---|---|
| `FLOWORK_TG_FORMAT` | html | format pesan keluar (html rapi / plain polos) |
| `FLOWORK_TG_MEDIA` | on | baca media masuk (off = text-only) |
| `FLOWORK_GROUP_SLASH` | off | slash group Telegram (lihat lock/group.md) |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_ALLOWED_CHATS` | — | secret (Settings) — token bot + chat owner |

---

## 5. ROUTING & OTAK (ringkas — detail di doc lain)

- **Route ke squad**: `task_list`→`task_run(category)` (anti-nyasar, lihat `lock/group.md`). Pre-classifier (`deterministicRoute` keyword + `classifyRoute` LLM) jalan SEBELUM `callLLM` di runDaemon + doHandle.
- **SELF-HANDLE GATE** (akar fix "disuruh sendiri malah nyalain crew"): kalau owner eksplisit nolak delegasi ("jangan pake agent/crew", "lakuin/kerjain sendiri") → `wantsSelfHandle()` true → **4 kondisi route di-SKIP** (`&& !wantsSelfHandle`) → jatuh ke `callLLM`, mr-flow kerjain SENDIRI (browser/web/file). Frasa di `self_handle_ext.go` (NON-frozen) + ENV `FLOWORK_SELF_HANDLE_PHRASES` (nambah tanpa buka freeze).
- **Anti-halu + akses internet**: web_search/webfetch + **browser asli** (akses penuh) + cek tahun, ga ngarang. Lihat persona block "ANTI-HALU".
- **Kontradiksi data**: `cognitive_tensions`/`cognitive_resolve` + tanya owner 3x/hari. Lihat `lock/CognitiveGraph.md`.
- **Persona** (kv `prompt`): identitas + ROUTER TEAM + ANTI-HALU + browser + kontradiksi. Edit AMAN: GET config UTUH → ubah `prompt` → POST (Save full-replace, secret ke-reconcile).

---

## 6. BUILD & DEPLOY (PENTING)

WASM = **tinygo** (wasi, `-scheduler=none -opt=z`) via `scripts/build-agent.sh` — toolchain deployed+proven (~499KB):
```
cd agent && GOWORK=off GOTOOLCHAIN=go1.23.4 bash scripts/build-agent.sh mr-flow
```
*(standard-go `GOOS=wasip1 GOARCH=wasm go build` JUGA compile (~4.8MB), tapi yang di-deploy = tinygo.)*
Deploy: copy `agent.wasm` → `~/.flowork/agents/mr-flow.fwagent/agent.wasm` (runtime yg kernel baca;
start.sh NEVER overwrite yg udah ada) → restart host (kill :1987, docktor rebuild) → kernel load wasm baru.
Edit main.go (frozen) butuh: chattr -i → edit → rebuild wasm → deploy → **re-hash KERNEL_FREEZE** → chattr +i.

---

## 7. PETA FILE & FREEZE

| File | Peran | Freeze |
|---|---|---|
| `agents/mr-flow/main.go` | core: long-poll, loop, LLM, sendMessage, struct Message, **seam #2C deferred-tools** (re-fetch specs abis `tool_lookup` → tool deferred masuk array, lihat `lock/tools.md §7.5`) | **FROZEN** brain-core (hash `26769416…`) |
| `agents/mr-flow/telegram_media.go` | CABANG: format + media handler + switch | NON-frozen |
| `agents/mr-flow/recall_gate.go`, `working_set.go`, `recovery_capture.go` | recall/context | (lihat status masing2) |
| `agent.wasm` | artifact build (gitignored) | — |
| Persona (kv `prompt`), tool_specs.go | enabler routing | non-frozen (sesekali tune) |

---

## 8. CARA NAMBAH FILTUR TELEGRAM (tanpa buka frozen)

- **Format / tipe media baru / vision / STT tweak** → `telegram_media.go` (cabang) + switch env.
- **Field Telegram baru** (mis. `sticker`, `location`) → tambah field di struct `Message` (main.go) BUTUH
  unfreeze (minim: cuma field), logic-nya di cabang. Minta izin owner buat unfreeze main.go.
- **Routing/persona** → kv `prompt` (data) / tool_specs (non-frozen).

---

## 8b. CATATAN PENDING / TODO (per 2026-06-23 — belum dikerjain, dicatat biar ga lupa)

- **Foto VISION belum penuh.** `visionDescribe` udah kirim format OpenAI `image_url` (data URI base64), TAPI
  router (`chatCompletionsHandler`) **belum pass content-array (image block) clean** ke model →
  foto sekarang kemungkinan cuma **acknowledge + caption**, belum bener-bener "dilihat". **TODO:** edit
  router biar preserve content array (image_url / Anthropic image block) + forward ke model vision-capable
  (Claude). Setelah itu "foto owner bisa dilihat" beneran jalan. Jalur Anthropic `/v1/messages`
  (`Content json.RawMessage`) lebih gampang preserve image — bisa dipakai sbg endpoint vision khusus.
- **Voice STT butuh provider AKTIF.** Jalan kalau owner set STT provider di Settings → Media Providers
  (deepgram/assemblyai/gemini/openai). Belum diset = mr-flow balas graceful note. **TODO:** cek/aktifin provider.
- **Dokumen binary (PDF/docx) belum dibaca isinya** — cuma teks (txt/md/code/json/csv). **TODO (opsional):**
  extractor PDF (di host-side tool, bukan WASM) kalau perlu.
- Semua di atas **GRACEFUL** sekarang (ga crash, selalu balas) — aman dipakai walau belum penuh.

### ARAH BESAR — buang subscription-gating (PROVEN mr-flow 2026-06-25; GLOBAL nunggu agentkit)
Owner usul: **buang gating subscription tool** (footgun "lupa centang GUI → agent lumpuh") → **SEMUA tool ke-expose nama-nya** (murah, lewat #2C deferred-katalog) + pilihan tool dikemudiin **DOKTRIN+INSTING+KONSTITUSI** (bukan allowlist statik). AMAN sebab **exposure ≠ permission**: tool bahaya tetep ke-gate cap pas RUN (`filterPrivilegedCaps` + `SandboxRun` Gate-1, INDEPENDEN subscription) — **divalidasi live + regression test 4/4**.

**STATUS mr-flow (SUDAH):**
- **Cap migrasi ke manifest** (langkah-3): 4 cap (`exec:shell`/`fs:read:/shared/*`/`fs:write:/shared/*`/`net:fetch:telegram`) ditambah ke `capabilities_required` (20→24) → mr-flow gak gantung subscription buat cap. Manifest re-frozen. (`net:fetch:telegram` KRITIS = I/O Telegram.)
- **All-tools ON** (langkah-4, switch `FLOWORK_DEFER_TOOLS`+`FLOWORK_EXPOSE_ALL_TOOLS`, scoped primary): mr-flow liat **202 tool** (22 schema + 180 katalog), tool non-sub bisa lookup+run, **Rule-9 LLM koheren NOL flail**. Agen lain gak kena.

**SISA (GLOBAL):** (1) ~~**agentkit**~~ ✅ **SELESAI 2026-06-25** — loop+guard+seam #2C ke-ekstrak ke modul SHARED `agent/agentkit/`; 5 worker + template warisan (verified Rule-9). Semua worker punya seam → all-tools GLOBAL ke-unblock. mr-flow tetep loop sendiri (referensi). Kanonik: `lock/agentkit.md`. (2) perkuat insting/konstitusi (roadmap #2/#2B) sbg kemudi pilih dari 200 tool; (3) GUI tool-catalog → repurpose ke kurasi doktrin/insting + toggle per-agent (ganti ENV). Detail tool: `lock/tools.md §7.5–7.6`.

---

## 9. PANTANGAN

- ❌ Jangan kirim pesan Telegram tanpa fallback polos (pesan ga boleh ilang gara2 HTML rusak).
- ❌ Jangan anggap STT/vision pasti ada — selalu graceful (provider bisa belum diset).
- ❌ Jangan print/log isi dokumen/foto owner ke tempat publik (privasi).
- ❌ Jangan buka main.go (frozen) buat filtur Telegram — pakai `telegram_media.go`.
- ❌ Jangan turunin cap fetch atau hapus fallback — bot owner = ga boleh diem/crash.
