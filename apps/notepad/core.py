#!/usr/bin/env python3
# core.py — Shared Notepad app CORE (ROADMAP 4, runtime:process). Bahasa: Python — BUKTI app
# lintas-bahasa. Protokol: baca {"op","args"} per baris di stdin, balas {"result","state_version"}
# per baris di stdout. State (catatan) ada di memori proses ini → human & agent edit yang SAMA.
import sys, json

state = {"text": ""}
ver = 0

def handle(op, args):
    global ver
    if op == "get":
        return {"result": dict(state), "state_version": ver}
    if op == "set":
        state["text"] = str(args.get("text", ""))
        ver += 1
        return {"result": {"ok": True, "text": state["text"]}, "state_version": ver}
    if op == "append":
        add = str(args.get("text", ""))
        state["text"] = (state["text"] + ("\n" if state["text"] else "") + add)
        ver += 1
        return {"result": {"ok": True, "text": state["text"]}, "state_version": ver}
    return {"error": "unknown op: " + str(op)}

def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            out = handle(req.get("op", ""), req.get("args") or {})
        except Exception as e:  # noqa
            out = {"error": str(e)}
        sys.stdout.write(json.dumps(out) + "\n")
        sys.stdout.flush()

if __name__ == "__main__":
    main()
