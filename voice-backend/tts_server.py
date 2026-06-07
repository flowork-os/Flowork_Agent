#!/usr/bin/env python3
"""Flowork sovereign TTS backend — free Microsoft Edge voices, no API key.

Speaks the edgeTts adapter's protocol from flow_router
(internal/providers/tts/edge_tts.go): POST /api/tts {text, voice, rate} -> mp3
bytes. The router's edgeTts provider is configured with baseUrl
http://127.0.0.1:5050, so this is the upstream it proxies to.

NOTE: edge-tts is free + no-key but calls Microsoft's online TTS service (not
fully offline). For 100%-offline TTS, swap this for piper later — the router
contract (POST /api/tts -> audio) stays the same, so nothing else changes.

Run: ~/.flowork/voice-backend/venv/bin/python tts_server.py
Env: TTS_PORT (default 5050), TTS_DEFAULT_VOICE (default id-ID-ArdiNeural)
"""
import os
import asyncio

import edge_tts
from aiohttp import web

DEFAULT_VOICE = os.environ.get("TTS_DEFAULT_VOICE", "id-ID-ArdiNeural")
PORT = int(os.environ.get("TTS_PORT", "5050"))


async def synth(text: str, voice: str, rate: str) -> bytes:
    """Render text to mp3 bytes via edge-tts (collect the audio stream)."""
    comm = edge_tts.Communicate(text, voice, rate=rate)
    chunks = []
    async for ev in comm.stream():
        if ev["type"] == "audio":
            chunks.append(ev["data"])
    return b"".join(chunks)


async def handle_tts(request: web.Request) -> web.Response:
    try:
        body = await request.json()
    except Exception as e:
        return web.json_response({"error": f"bad json: {e}"}, status=400)
    text = (body.get("text") or "").strip()
    if not text:
        return web.json_response({"error": "text required"}, status=400)
    voice = body.get("voice") or DEFAULT_VOICE
    rate = body.get("rate") or "+0%"
    try:
        audio = await synth(text, voice, rate)
    except Exception as e:
        return web.json_response({"error": f"tts: {e}"}, status=502)
    if not audio:
        return web.json_response({"error": "empty audio"}, status=502)
    return web.Response(body=audio, content_type="audio/mpeg")


async def handle_health(_request: web.Request) -> web.Response:
    return web.json_response({"ok": True, "service": "flowork-tts", "voice": DEFAULT_VOICE})


def main():
    app = web.Application()
    app.router.add_post("/api/tts", handle_tts)
    app.router.add_get("/health", handle_health)
    print(f"[flowork-tts] edge-tts server on http://127.0.0.1:{PORT} (voice={DEFAULT_VOICE})", flush=True)
    web.run_app(app, host="127.0.0.1", port=PORT, print=None)


if __name__ == "__main__":
    main()
