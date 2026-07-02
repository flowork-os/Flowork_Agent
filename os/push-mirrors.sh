#!/usr/bin/env bash
# push-mirrors.sh — mirror monorepo HEAD → repo SEO (Flowork_Agent + flowork_Router),
# TAPI pertahankan README + gambar tiap repo (biar halaman ranked / SEO ga ilang).
# README+aset per-repo disimpen di os/mirror-readme/<repo>/ (version-controlled).
# Repo lama = mirror penuh monorepo (4 backup identik) + README-nya sendiri.
#
# Token: $FLOWORK_GH_TOKEN, else grep dari $FLOWORK_SECRETS/GITHUB_ACCOUNT.MD
# (default $HOME/Documents/flowork-secrets). Remote agent-mirror/router-mirror harus
# udah ke-add (git remote add agent-mirror https://github.com/flowork-os/Flowork_Agent, dst).
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

SECRETS="${FLOWORK_SECRETS:-$HOME/Documents/flowork-secrets}"
TOKEN="${FLOWORK_GH_TOKEN:-$(grep -oE 'gh[ps]_[A-Za-z0-9]{36,}' "$SECRETS/GITHUB_ACCOUNT.MD" 2>/dev/null | head -1)}"
[ -n "$TOKEN" ] || { echo "no GitHub token (set FLOWORK_GH_TOKEN)"; exit 1; }
B64="$(printf 'x-access-token:%s' "$TOKEN" | base64 -w0)"

# "repo-name remote-name" — repo SEO yang di-mirror.
MIRRORS=("Flowork_Agent agent-mirror" "flowork_Router router-mirror")

for entry in "${MIRRORS[@]}"; do
	set -- $entry; repo="$1"; remote="$2"
	git remote get-url "$remote" >/dev/null 2>&1 || {
		echo "SKIP $repo: remote '$remote' belum ke-add"; continue; }
	ov="$REPO/os/mirror-readme/$repo"
	wt="/tmp/wt-mirror-$repo"; br="mirror-tmp-$repo"
	git worktree remove --force "$wt" 2>/dev/null || true
	git branch -D "$br" 2>/dev/null || true
	git worktree add -q -b "$br" "$wt" HEAD
	[ -d "$ov" ] && cp -a "$ov/." "$wt/"   # overlay README+aset repo (SEO)
	git -C "$wt" add -A
	git -C "$wt" commit -q -m "chore: mirror monorepo into $repo (keep README+assets for SEO)" || true
	git -C "$wt" -c http.extraHeader="Authorization: Basic $B64" push --force "$remote" "$br:main"
	git worktree remove --force "$wt"; git branch -D "$br" 2>/dev/null || true
	echo "mirrored → $repo (main)"
done
echo "DONE mirrors."
