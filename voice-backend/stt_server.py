#!/usr/bin/env python3
# === LOCKED FILE ===
# Status: STABLE — Flowork voice backend, live-proven 2026-06-07. Do not edit without owner approval.
# Owner: Aola Sahidin (Mr.Dev).
"""Flowork sovereign STT backend — 100%% local/offline via faster-whisper.

Speaks the OpenAI-compatible transcription protocol the flow_router "openai" STT
adapter expects (internal/providers/stt/openai_whisper.go): multipart POST to
/v1/audio/transcriptions with a "file" field -> JSON {text, language, duration}.
The router's STT provider is configured with provider="openai" +
baseUrl=http://127.0.0.1:5060/v1, so this is the upstream it calls.

Fully local: faster-whisper runs the model on CPU (int8). The model is
downloaded once on first run, then cached — afterwards it works offline.

Run: ~/.flowork/voice-backend/venv/bin/python stt_server.py
Env: STT_PORT (default 5060), STT_MODEL (default base), STT_COMPUTE (default int8)
"""
import os
import tempfile

from aiohttp import web
from faster_whisper import WhisperModel

PORT = int(os.environ.get("STT_PORT", "5060"))
MODEL_NAME = os.environ.get("STT_MODEL", "base")
COMPUTE = os.environ.get("STT_COMPUTE", "int8")

print(f"[flowork-stt] loading faster-whisper model={MODEL_NAME} compute={COMPUTE} (first run downloads it)…", flush=True)
MODEL = WhisperModel(MODEL_NAME, device="cpu", compute_type=COMPUTE)
print("[flowork-stt] model ready", flush=True)


async def handle_transcribe(request: web.Request) -> web.Response:
    try:
        reader = await request.multipart()
    except Exception as e:
        return web.json_response({"error": f"not multipart: {e}"}, status=400)

    audio_bytes = b""
    language = None
    async for part in reader:
        if part.name == "file":
            audio_bytes = await part.read(decode=False)
        elif part.name == "language":
            language = (await part.text()).strip() or None
        else:
            await part.read()  # drain model/response_format/etc.

    if not audio_bytes:
        return web.json_response({"error": "missing 'file' field"}, status=400)

    # faster-whisper reads a path; write the upload to a temp file.
    with tempfile.NamedTemporaryFile(suffix=".audio", delete=True) as tf:
        tf.write(audio_bytes)
        tf.flush()
        try:
            segments, info = MODEL.transcribe(tf.name, language=language, vad_filter=True)
            text = "".join(seg.text for seg in segments).strip()
        except Exception as e:
            return web.json_response({"error": f"transcribe: {e}"}, status=502)

    return web.json_response({
        "text": text,
        "language": getattr(info, "language", "") or "",
        "duration": getattr(info, "duration", 0.0) or 0.0,
    })


async def handle_health(_request: web.Request) -> web.Response:
    return web.json_response({"ok": True, "service": "flowork-stt", "model": MODEL_NAME})


def main():
    app = web.Application(client_max_size=64 * 1024 * 1024)  # 64 MiB audio cap
    # Serve both with and without the /v1 prefix so any baseUrl shape works.
    app.router.add_post("/v1/audio/transcriptions", handle_transcribe)
    app.router.add_post("/audio/transcriptions", handle_transcribe)
    app.router.add_get("/health", handle_health)
    print(f"[flowork-stt] server on http://127.0.0.1:{PORT}", flush=True)
    web.run_app(app, host="127.0.0.1", port=PORT, print=None)


if __name__ == "__main__":
    main()
