# Flowork Voice Backend (sovereign, optional)

Two tiny local services that give Flowork **voice** — speech-to-text (STT) and
text-to-speech (TTS) — without any paid cloud key. They are **optional**: the
voice feature is plug-and-play through the router, so you can instead point the
router's Media Providers at a cloud STT/TTS provider (works on any OS, zero local
install). This folder is the **sovereign / offline-leaning** option.

## What runs

| Service | Port | Engine | Sovereignty |
|---|---|---|---|
| STT | 5060 | [faster-whisper](https://github.com/SYSTRAN/faster-whisper) | **100% local/offline** (model cached after first run) |
| TTS | 5050 | [edge-tts](https://github.com/rany2/edge-tts) | free + no key (calls Microsoft's online voices — not fully offline; swap for piper later for 100% offline) |

The router proxies to these: it exposes an OpenAI-compatible voice API
(`/v1/audio/transcriptions`, `/api/media-providers/tts`) and the **channel** (e.g.
Telegram) just calls the router — it never talks to these services directly. One
backend swap = nothing else changes.

## Setup (multi-OS: Linux / macOS / Windows)

```bash
python3 -m venv venv
venv/bin/pip install edge-tts faster-whisper aiohttp        # Windows: venv\Scripts\pip
```

## Run

```bash
# Linux/macOS
./start-voice.sh
# Any OS (cross-platform launcher)
venv/bin/python start_voice.py        # Windows: venv\Scripts\python start_voice.py
```

Both launchers are idempotent (skip a service whose port is already up) and
detach the services. Logs + PIDs land beside the scripts.

## Wire it into the router (once)

Add two Media Providers in flow_router (GUI, or `POST /api/media-providers`):

```jsonc
// STT — local whisper
{ "category":"stt", "provider":"openai", "baseUrl":"http://127.0.0.1:5060/v1", "apiKey":"local", "models":["base"], "isActive":true }
// TTS — edge-tts (leave baseUrl EMPTY so the router uses its in-process adapter)
{ "category":"tts", "provider":"edgeTts", "baseUrl":"", "models":["edgeTts"], "isActive":true }
```

Env knobs: `STT_MODEL` (default `base`; `small` is more accurate, slower),
`TTS_DEFAULT_VOICE` (default `id-ID-ArdiNeural`), `TTS_PORT`, `STT_PORT`.

## Verify

```bash
# TTS: text -> mp3
curl -s -XPOST http://127.0.0.1:2402/api/media-providers/tts \
  -H 'Content-Type: application/json' \
  -d '{"text":"halo flowork","voice":"id-ID-ArdiNeural","format":"mp3"}' -o out.mp3
# STT: mp3 -> text
curl -s -XPOST http://127.0.0.1:2402/v1/audio/transcriptions \
  -F file=@out.mp3 -F model=base -F language=id
```

Remove the whole feature by deleting this folder + the two Media Providers — the
rest of Flowork is untouched.
