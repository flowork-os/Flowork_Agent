#!/usr/bin/env bash
# push-all.sh — SATU perintah push monorepo main ke SEMUA 4 repo:
#   canonical : origin (Flowork-OS, publik) + flowork-base (privat backup)
#   SEO mirror: Flowork_Agent + flowork_Router (via push-mirrors.sh, README-nya kejaga)
#
# Tujuan: hemat ngurus — ga usah push satu-satu ke 4 tempat. Jalanin dari mana aja:
#   bash os/push-all.sh
# Token: $FLOWORK_GH_TOKEN, else $FLOWORK_SECRETS/GITHUB_ACCOUNT.MD (default ~/Documents/flowork-secrets).
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

SECRETS="${FLOWORK_SECRETS:-$HOME/Documents/flowork-secrets}"
TOKEN="${FLOWORK_GH_TOKEN:-$(grep -oE 'gh[ps]_[A-Za-z0-9]{36,}' "$SECRETS/GITHUB_ACCOUNT.MD" 2>/dev/null | head -1)}"
[ -n "$TOKEN" ] || { echo "no GitHub token (set FLOWORK_GH_TOKEN)"; exit 1; }
B64="$(printf 'x-access-token:%s' "$TOKEN" | base64 -w0)"

# 1) canonical: fast-forward push (JANGAN force — auto-update user baca dari sini).
for remote in origin flowork-base; do
	git remote get-url "$remote" >/dev/null 2>&1 || { echo "SKIP $remote (belum ke-add)"; continue; }
	git -c http.extraHeader="Authorization: Basic $B64" push "$remote" main
	echo "pushed → $remote (main)"
done

# 2) SEO mirror (README-preserving force-mirror).
bash "$REPO/os/push-mirrors.sh"

echo "ALL 4 repos updated."
