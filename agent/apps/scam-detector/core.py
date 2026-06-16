#!/usr/bin/env python3
# core.py — Scam Detector app CORE (Flowork apps platform, runtime:process).
#
# WHAT IT DOES (anti-hallucination by design — "kecerdasan dulu"):
#   1. GROUND: call the check_token tool for FACTUAL on-chain data (GoPlus for EVM,
#      RugCheck for Solana). This is deterministic truth, no LLM guessing.
#   2. DEBATE: hand that factual report to the `scam-detector` agent GROUP (colony):
#      on-chain analyst + scam-pattern analyst debate, a synthesizer fuses ONE verdict.
#   3. Return verdict + the raw factual flags to the GUI.
#
# The group is reached through the SAME kernel/agent HTTP surface the rest of Flowork
# uses, so the app stays a thin orchestrator — all intelligence lives in the colony.
#
# Protocol: read {"op","args"} per line on stdin, reply {"result", ...}/{"error"} per
# line on stdout (Flowork apps platform contract — same as the notepad sample).
import sys, json, os, urllib.request, urllib.error, urllib.parse

# Endpoints are overridable (no hardcode of behavior): point the app at another host
# or swap the grounding agent / group without touching code.
AGENT_BASE   = os.environ.get("FLOWORK_AGENT_URL", "http://127.0.0.1:1987").rstrip("/")
GROUND_AGENT = os.environ.get("SCAM_GROUND_AGENT", "scam-detector-onchain")  # holds net:fetch:*
GROUP_ID     = os.environ.get("SCAM_GROUP", "scam-detector")
HTTP_TIMEOUT = int(os.environ.get("SCAM_HTTP_TIMEOUT", "110"))


def _post(path, payload):
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(AGENT_BASE + path, data=data,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as r:
        return json.loads(r.read().decode("utf-8"))


def check_token(chain, address):
    """FACTUAL grounding — run the check_token tool via a capability-holding agent."""
    r = _post("/api/agents/tools/run?id=" + urllib.parse.quote(GROUND_AGENT),
              {"tool_name": "check_token",
               "args": {"chain": chain, "address": address},
               "caller": "scam-detector-app"})
    if not r.get("ok"):
        raise RuntimeError(r.get("error", "check_token failed"))
    out = (r.get("result") or {}).get("output")
    if not isinstance(out, dict):
        raise RuntimeError("check_token returned no report")
    return out


def group_verdict(chain, address, report):
    """Hand the FACTUAL report to the scam-detector colony; return the fused verdict."""
    text = ("Token: %s on %s.\n"
            "FACTUAL security report (analyze THIS DATA ONLY, never invent beyond it):\n%s\n"
            "Assess scam/rug risk and return one verdict."
            % (address, chain, json.dumps(report, ensure_ascii=False)))
    r = _post("/api/kernel/rpc",
              {"plugin": GROUP_ID, "function": "handle_message", "args": {"text": text}})
    if r.get("error"):
        raise RuntimeError(r["error"])
    return (r.get("reply") or "").strip()


def handle(op, args):
    if op == "scan_token":
        chain = str(args.get("chain", "")).strip().lower()
        address = str(args.get("address", "")).strip()
        if not chain or not address:
            return {"error": "chain and address are required"}
        try:
            report = check_token(chain, address)
        except Exception as e:  # grounding is mandatory — no factual data, no verdict
            return {"error": "on-chain grounding failed: " + str(e)}
        try:
            verdict = group_verdict(chain, address, report)
        except Exception as e:  # colony down -> still return the factual report, flag it
            verdict = "(agent colony unavailable — showing factual data only: %s)" % e
        return {"result": {
            "chain": chain,
            "address": address,
            "source": report.get("source"),
            "risk_level": report.get("risk_level"),
            "score": report.get("score"),
            "flags": report.get("flags", []),
            "summary": report.get("summary"),
            "verdict": verdict,
        }}
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
