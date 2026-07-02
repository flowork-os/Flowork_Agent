#!/usr/bin/env python3
"""sanitize-public.py — turn a Flowork config/seed JSON into a PUBLIC-safe one.

Every secret becomes the literal placeholder `change_this_token`, so a public build can be
shared / committed and the recipient just fills in their own values. Two passes:

  1. Key-based: any object key that looks like a secret (apiKey, token, secret, password,
     cookie, credential, *_key, authorization, …) has its string value replaced.
  2. Pattern-based: any string value anywhere that matches a known secret shape (GitHub
     ghp_*, OpenAI sk-*, Telegram bot<digits>:*, Slack xox*-*, AWS AKIA*, JWT, PEM block,
     or the old PASTE_YOUR_KEY_HERE placeholder) is replaced — a safety net for stray secrets.

Usage:  sanitize-public.py <in.json> <out.json>
        sanitize-public.py --selftest
"""
import json, re, sys

PLACEHOLDER = "change_this_token"

SECRET_KEY = re.compile(
    r"(api[_-]?key|secret|password|passwd|token|cookie|credential|client[_-]?secret|"
    r"private[_-]?key|access[_-]?key|auth(oriz(ation|e))?|bearer|x_auth_token|x_ct0|"
    r"bot[_-]?token|chat[_-]?id|refresh[_-]?token|oauth|.*[_-]?key|.*[kK]ey)$",
    re.IGNORECASE,
)

SECRET_PATTERNS = [
    re.compile(r"PASTE_YOUR_KEY_HERE"),
    re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    re.compile(r"github_pat_[A-Za-z0-9_]{20,}"),
    re.compile(r"sk-[A-Za-z0-9_\-]{20,}"),
    re.compile(r"xox[baprs]-[A-Za-z0-9-]{10,}"),
    re.compile(r"\bbot\d{6,}:[A-Za-z0-9_\-]{20,}"),   # telegram bot token
    re.compile(r"AKIA[0-9A-Z]{16}"),                  # aws access key id
    re.compile(r"eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}"),  # jwt
    re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    # Google / Gemini / Antigravity OAuth (audit 2026-07-02): provider aktif Antigravity pakai
    # OAuth Google → tutup celah kalau creds-nya kelak masuk file yg di-sanitize.
    re.compile(r"AIza[0-9A-Za-z_\-]{35}"),            # google api key (gemini/maps)
    re.compile(r"ya29\.[0-9A-Za-z_\-]{20,}"),         # google oauth access token
    re.compile(r"1//[0-9A-Za-z_\-]{20,}"),            # google oauth refresh token
]


def looks_secret_value(s: str) -> bool:
    return any(p.search(s) for p in SECRET_PATTERNS)


def scrub(node, parent_key=None):
    if isinstance(node, dict):
        return {k: scrub(v, k) for k, v in node.items()}
    if isinstance(node, list):
        return [scrub(v, parent_key) for v in node]
    if isinstance(node, str):
        if parent_key and SECRET_KEY.search(parent_key) and node.strip():
            return PLACEHOLDER
        if looks_secret_value(node):
            return PLACEHOLDER
    return node


def sanitize_text(s: str) -> str:
    try:
        return json.dumps(scrub(json.loads(s)), indent=2, ensure_ascii=False)
    except json.JSONDecodeError:
        # Not JSON — fall back to pattern-only scrub so we still never leak a secret.
        for p in SECRET_PATTERNS:
            s = p.sub(PLACEHOLDER, s)
        return s


def selftest():
    sample = {
        "providers": [{
            "name": "Claude",
            "apiKey": "sk-ant-REALSECRETvalue1234567890",
            "data": {"tokenSource": "claude_credentials", "model": "claude-haiku-4-5"},
        }],
        "blanked": "PASTE_YOUR_KEY_HERE",
        "telegram": {"bot_token": "bot1234567890:AAEReal-Telegram-Token_xyz123456"},
        "github_token": "ghp_EXAMPLEfake0000000000000000000000000",
        "public_url": "https://example.com/ok",   # must stay
        "note": "this is fine",                    # must stay
    }
    out = json.loads(sanitize_text(json.dumps(sample)))
    assert out["providers"][0]["apiKey"] == PLACEHOLDER, "apiKey not scrubbed"
    assert out["providers"][0]["data"]["model"] == "claude-haiku-4-5", "model wrongly scrubbed"
    assert out["blanked"] == PLACEHOLDER, "placeholder not normalised"
    assert out["telegram"]["bot_token"] == PLACEHOLDER, "telegram token leaked"
    assert out["github_token"] == PLACEHOLDER, "github token leaked"
    assert out["public_url"] == "https://example.com/ok", "public url wrongly scrubbed"
    assert out["note"] == "this is fine", "note wrongly scrubbed"
    blob = json.dumps(out)
    for bad in ("sk-ant-REAL", "ghp_EXAMPLEfake", "AAEReal-Telegram", "PASTE_YOUR_KEY_HERE"):
        assert bad not in blob, f"secret leaked: {bad}"
    print("sanitize-public selftest: OK (all secrets -> change_this_token, public data kept)")


def main():
    if len(sys.argv) == 2 and sys.argv[1] == "--selftest":
        selftest(); return
    if len(sys.argv) != 3:
        print(__doc__); sys.exit(2)
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        text = f.read()
    with open(sys.argv[2], "w", encoding="utf-8") as f:
        f.write(sanitize_text(text) + "\n")
    print(f"sanitized -> {sys.argv[2]}  (secrets -> {PLACEHOLDER})")


if __name__ == "__main__":
    main()
