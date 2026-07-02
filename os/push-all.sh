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
# Pilih token yg BENERAN punya akses PUSH (GITHUB_ACCOUNT.MD punya 2 token: flowork-dev
# read-only + flowork-os owner). `head -1` rapuh — urutan file bisa berubah → ke-grab yg
# read-only → 403. Jadi tes tiap token, ambil yg push=true ke Flowork-OS. Fallback head -1.
_pick_push_token() {
	local f="$SECRETS/GITHUB_ACCOUNT.MD" t
	for t in $(grep -oE 'gh[ps]_[A-Za-z0-9]{36,}' "$f" 2>/dev/null); do
		if curl -s -m 8 -H "Authorization: Bearer $t" \
			https://api.github.com/repos/flowork-os/Flowork-OS 2>/dev/null | grep -q '"push": *true'; then
			printf '%s' "$t"; return 0
		fi
	done
	grep -oE 'gh[ps]_[A-Za-z0-9]{36,}' "$f" 2>/dev/null | head -1
}
TOKEN="${FLOWORK_GH_TOKEN:-$(_pick_push_token)}"
[ -n "$TOKEN" ] || { echo "no GitHub token (set FLOWORK_GH_TOKEN)"; exit 1; }
export FLOWORK_GH_TOKEN="$TOKEN"   # push-mirrors.sh reuse token yg sama
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
