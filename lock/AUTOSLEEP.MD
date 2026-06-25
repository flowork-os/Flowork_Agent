# AUTOSLEEP — LLM Idle-Sleep Power Saver (hemat GPU/RAM, PC adem)

> Dokumen arsitektur. Owner: Aola Sahidin (Mr.Dev). Dibuat 2026-06-25.
> Fitur: model LLM lokal (`flowork-brain.gguf` ~14GB) **di-unload** dari GPU/RAM saat
> nganggur, **di-reload otomatis** saat dipakai. Tujuan: PC gak panas pas idle.
> Sejalan doktrin **AOLA-008 (Resource Ephemerality)** — resource sementara dihancurkan saat selesai.

---

## 0. MASALAH → AKAR (cabut akar, bukan tambal)

`llama-server` (:8088, model 14GB MoE, `-ngl 99`) nyala **24/7** walau gak ada yang chat →
GPU/VRAM/RAM kepake terus → PC panas + boros listrik. Akar: engine lokal gak punya konsep
"tidur saat idle". Solusi: bikin engine **EFEMERAL** — unload saat idle, reload on-demand.

---

## 1. INVARIAN KUNCI (kenapa aman dengan SELURUH arsitektur)

**Router `:2402` = SATU-SATUNYA gerbang ke engine lokal `:8088`.**
- SEMUA konsumen LLM — chat interaktif DAN semua job background (schedule, trigger, spawn,
  dream/digest, learning) — manggil model lewat `routerclient` → `http://127.0.0.1:2402/v1/chat/completions`
  → `DispatchChatCompletion` / `DispatchChatCompletionStream`.
- **TIDAK ADA** pemanggil `:8088` langsung (audit grep: satu-satunya rujukan `:8088` di luar
  dispatcher = `main.go` deteksi-provider autostart, bukan request). Agent **gak pernah** nembak
  llama-server langsung.

→ Cukup taruh **satu gate** di jalur dispatch itu, maka **semua** jalur (interaktif + background)
otomatis ke-cover. Gak ada job yang "mati" gara-gara engine tidur.

```
 chat user ─┐
 dream/digest (cognitive_digest_cron → routerclient) ─┐
 learning / mistake-promote / recovery ─┐             │
 schedule / wakeup / trigger / spawn (task_worker) ─┐ │
                                                    ▼ ▼
                         routerclient → ROUTER :2402 → DispatchChatCompletion(Stream)
                                                    │
                                      defer wakeLocalIfNeeded(baseURL)()   ← GATE (gated :8088)
                                                    │
                                              llama-server :8088 (wake kalau tidur)
```

---

## 2. DESAIN (nano-modular, gak bongkar file locked)

Dua file BARU + tiga hook satu-baris. Engine-core (`localai/runtime.go`) **soft-locked → TIDAK disentuh**;
fitur ini cuma manggil API publiknya (`Start`/`Stop`/`Status`).

| File | Peran | Status |
|---|---|---|
| `router/llm_idle_sleep.go` (pkg main) | manager: watchdog unload + wake + in-flight guard + config | BARU, non-locked |
| `router/internal/router/llm_wake_hook.go` (pkg router) | seam `LocalLLMGate` (begin→end), `wakeLocalIfNeeded` | BARU, non-locked |
| `router/internal/router/dispatcher.go` | `defer wakeLocalIfNeeded(baseURL)()` di `forwardToProvider` | LOCKED, +1 baris (owner-approved) |
| `router/internal/router/dispatcher_stream.go` | idem di `streamFromProvider` | LOCKED, +1 baris |
| `router/main.go` | `llmIdleSleepInit()` di boot | LOCKED, +1 baris |

**Dua mekanisme:**
- **Watchdog** (ticker 30s): kalau engine idle > timeout **DAN** `in-flight == 0` → `Runtime.Stop()`.
- **Gate** (`defer wakeLocalIfNeeded(baseURL)()`): tiap request lokal → wake engine kalau tidur,
  daftar in-flight, lepas saat request balik.

---

## 3. ANTI-ZOMBIE (jaminan gak ada proses nyangkut di belakang)

1. **Reap bersih.** `Runtime.Stop()` = `Process.Kill()` + `Process.Wait()`. `Wait()` nge-reap →
   **gak ada defunct/zombie**. Wrapper `router/bin/llama-server` pakai `exec llama-server.real`
   → PID dipertahankan → Kill kena engine asli langsung (bukan bunuh wrapper doang).
2. **Gak ada orphan.** Engine selalu child dari proses router (PPID=router). Pas router di-restart,
   systemd `stop` bunuh seluruh cgroup `flowork-docktor.service` → engine ikut mati.
3. **In-flight guard.** Watchdog **gak akan** unload selama ada request terbuka (`in-flight > 0`),
   termasuk job background yang lama → gak ada request keputus di tengah.
4. **Anti double-reload.** `llmWakeMu` nge-serialize reload → N request barengan saat tidur =
   **1** cold-reload (bukan N). Terbukti: 3 request paralel → 1 reload, jawaban benar semua.
5. **Self-heal gak berantem.** `os/selfheal/watchdog.sh` cuma jaga `:2402` (router) & `:1987` (gui),
   **TIDAK** `:8088`. Jadi engine tidur ≠ dianggap crash → gak di-restart paksa.
6. **Reconcile.** Tiap tick watchdog nyamain cache `awake` dgn status proses nyata; engine yang
   crash di luar (defunct) ke-reap di wake/stop berikutnya dalam ≤30s.

---

## 4. PERILAKU & TRADE-OFF

- **Wake latency**: ~15–25 dtk sekali (cold-reload 14GB MoE) tiap engine bangun dari tidur.
  Request yang mancing wake yang nanggung delay ini; sesudahnya cepat normal.
- **Idle window default 120 dtk** (2 menit). Background job yang jalan < 2 mnt sekali bikin engine
  tetap bangun; idle beneran (gak ada chat/job) → tidur → VRAM bebas (terbukti `nvidia-smi`).
- Engine embedding (ollama `:11434`, kecil) **di luar scope** — dipakai recall/embedding yang
  sering & ringan; `:8088` (model gede) doang yang di-sleep.

---

## 5. KONFIG (env, di `router/flowork.local.env` — per-mesin, gitignored)

```
export FLOWORK_LLM_IDLE_SLEEP_SEC=120   # detik idle sebelum unload (default 120; min 10)
# export FLOWORK_LLM_IDLE_SLEEP=0        # MATIIN fitur (engine nyala terus kayak dulu)
```

Log boot: `LLM idle-sleep: ON — unload flowork-brain after 2m0s idle, reload on demand`.

---

## 6. BUKTI UJI (2026-06-25, live)

- Idle → `unloaded flowork-brain … GPU/RAM freed` + `nvidia-smi`: flowork-brain **BEBAS** dari VRAM.
- Request → `waking flowork-brain (cold reload)` → `UP on :8088` → jawab benar (~21 dtk).
- 3 request paralel saat tidur → **1** cold-reload, 3 jawaban benar.
- Higiene: **1** instance engine (PPID=router), **0** defunct/zombie, teardown lepas VRAM bersih.
- Background (digest/dream/learning/schedule) → `routerclient` → `:2402` → kena gate (no bypass).

---

## 7. FREEZE (chattr +i + manifest) — 2026-06-25

Yang di-**FREEZE** (immutable OS + hash di `flowork-secrets/KERNEL_FREEZE.md`, total 153 hash):

| File | Kenapa frozen |
|---|---|
| `router/llm_idle_sleep.go` | manager inti (watchdog/wake/in-flight/switch) — jantung fitur |
| `router/internal/router/llm_wake_hook.go` | seam dispatcher (`LocalLLMGate`) — "jalur"-nya |

Yang **TIDAK di-freeze** (sengaja, biar arsitektur tetap bisa tumbuh — Rule 7):
- `router/llm_idle_sleep_ext.go` — **growth-point** (titik-tumbuh fitur baru, lihat §8).
- Hook 1-baris di `dispatcher.go` / `dispatcher_stream.go` / `main.go` — **soft-lock** (NON-chattr).
  Logic-nya udah di file frozen; di sini cuma `defer wakeLocalIfNeeded(baseURL)()` / `llmIdleSleepInit()`
  yang stabil. Host tetap editable buat hal lain (pola FASE-C `lock/brain.md`: cabang → file terpisah frozen, host thin).

**Bukti freeze (CARAFREEZE.MD):** `chattr +i` → `lsattr` = `----i---------e-------`; `echo x >> file`
→ **"Operation not permitted"**; `cd agent && GOWORK=off go test -run TestKernelFreeze` → **PASS**.
**Unfreeze sadar** (kalau owner mau ubah): `sudo chattr -i <file>` → edit → re-`sha256sum` → update
`KERNEL_FREEZE.md` → `sudo chattr +i` → `TestKernelFreeze` PASS. ⚠️ Dev-user punya passwordless-sudo →
freeze = proteksi anti-edit-**tak-sengaja** (sesuai tujuan), bukan anti-AI-jahat (hardening N3: user non-sudo).

---

## 8. SWITCH — cara nambah fitur TANPA buka freeze (PENTING)

Inti-nya beku, tapi **bisa di-extend** lewat 4 switch yang sengaja di-expose (pola sama
`MemTypeClassifierHook` di `brain_drawers.go`, 2026-06-23). **Jangan unfreeze** — bikin **file BARU**
(non-frozen, mis. `feature_xxx.go`) yang manggil switch ini:

| Switch (di file frozen) | Buat apa |
|---|---|
| `RegisterLLMLifecycleObserver(onWake, onSleep)` | react pas engine bangun/tidur (GUI status, metrics, ikut tidurin embedding, notif) — observer di-recover, panik gak matiin watchdog |
| `SetLLMIdleConfig(enabled, timeoutSec)` | toggle on/off + ganti timeout **runtime** (mis. tombol GUI), tanpa restart |
| `LLMIdleStatus() (enabled,timeoutSec,awake,inFlight)` | baca state buat GUI/diagnostic |
| `router.LocalLLMGate` (lanjutan) | bungkus gate per-request (advanced; set sesudah `llmIdleSleepInit`) |

**Template (copy ke file baru, NOL sentuh freeze):**
```go
// router/feature_autosleep_xxx.go (package main, non-frozen)
func init() {
    RegisterLLMLifecycleObserver(
        func() { /* on WAKE: nyalain indikator / mulai metrics */ },
        func() { /* on SLEEP: ikut unload engine embedding / kirim notif */ },
    )
}
```
File contoh + smoke-test: `router/llm_idle_sleep_ext.go` (env `FLOWORK_LLM_IDLE_DEBUG=1` → observer nge-log;
kebukti `engine WOKE`/`engine SLEPT` muncul saat siklus). Jadi **fitur baru = file baru**, inti beku tetap aman.
