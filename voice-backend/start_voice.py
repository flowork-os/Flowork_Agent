#!/usr/bin/env python3
"""Cross-platform launcher for the Flowork sovereign voice backend.

Works on Windows / macOS / Linux (no bash needed) — the multi-OS counterpart to
start-voice.sh. Starts the two local services flow_router proxies to:
  - TTS on :5050 (edge-tts, free)            -> tts_server.py
  - STT on :5060 (faster-whisper, offline)   -> stt_server.py

Idempotent: skips a service whose port is already listening. Detached so it
keeps running after this launcher exits. Logs + PIDs live beside this file.

Run with the venv interpreter, e.g.:
  Linux/macOS:  ~/.flowork/voice-backend/venv/bin/python start_voice.py
  Windows:      %USERPROFILE%\\.flowork\\voice-backend\\venv\\Scripts\\python start_voice.py
"""
import os
import sys
import socket
import subprocess

HERE = os.path.dirname(os.path.abspath(__file__))
TTS_PORT = int(os.environ.get("TTS_PORT", "5050"))
STT_PORT = int(os.environ.get("STT_PORT", "5060"))


def port_busy(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.5)
        return s.connect_ex(("127.0.0.1", port)) == 0


def spawn(name: str, script: str, port: int):
    if port_busy(port):
        print(f"[{name}] port {port} already in use — skip")
        return
    log = open(os.path.join(HERE, f"{name}.log"), "ab")
    # Detach: new session/process-group so it outlives this launcher.
    kwargs = {}
    if os.name == "posix":
        kwargs["start_new_session"] = True
    else:  # Windows
        kwargs["creationflags"] = 0x00000008 | 0x00000200  # DETACHED|NEW_PROCESS_GROUP
    p = subprocess.Popen(
        [sys.executable, os.path.join(HERE, script)],
        stdout=log, stderr=log, stdin=subprocess.DEVNULL,
        env={**os.environ, "TTS_PORT": str(TTS_PORT), "STT_PORT": str(STT_PORT)},
        **kwargs,
    )
    with open(os.path.join(HERE, f"{name}.pid"), "w") as f:
        f.write(str(p.pid))
    print(f"[{name}] started (pid {p.pid}, port {port})")


def main():
    spawn("tts", "tts_server.py", TTS_PORT)
    spawn("stt", "stt_server.py", STT_PORT)
    print(f"voice backend up — TTS :{TTS_PORT} · STT :{STT_PORT}")


if __name__ == "__main__":
    main()
