#!/usr/bin/env python3
# core.py — Shared Notepad app CORE (ROADMAP 4, runtime:process). Bahasa: Python — BUKTI app
# lintas-bahasa. Protokol: baca {"op","args"} per baris di stdin, balas {"result","state_version"}
# per baris di stdout. State (catatan) ada di memori proses ini → human & agent edit yang SAMA.
#
# PERSISTENCE: core nyimpen state ke state.json di FOLDER-NYA SENDIRI (cwd = app dir, di-set
# host via cmd.Dir). Jadi catatan TETEP ADA lintas restart Flowork. Pola ini = tanggung jawab
# app (host ga maksa skema) → app bahasa apa pun bebas pilih cara persist (file/db/dll).
import sys, json, os

STATE_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "state.json")

def load():
    try:
        with open(STATE_FILE, "r", encoding="utf-8") as f:
            d = json.load(f)
            return {"text": str(d.get("text", ""))}, int(d.get("ver", 0))
    except Exception:  # noqa
        return {"text": ""}, 0

state, ver = load()

def persist():
    try:
        tmp = STATE_FILE + ".tmp"
        with open(tmp, "w", encoding="utf-8") as f:
            json.dump({"text": state["text"], "ver": ver}, f)
        os.replace(tmp, STATE_FILE)  # atomic — anti file korup kalau mati di tengah
    except Exception:  # noqa
        pass

def handle(op, args):
    global ver
    if op == "get":
        return {"result": dict(state), "state_version": ver}
    if op == "set":
        state["text"] = str(args.get("text", ""))
        ver += 1
        persist()
        return {"result": {"ok": True, "text": state["text"]}, "state_version": ver}
    if op == "append":
        add = str(args.get("text", ""))
        state["text"] = (state["text"] + ("\n" if state["text"] else "") + add)
        ver += 1
        persist()
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
