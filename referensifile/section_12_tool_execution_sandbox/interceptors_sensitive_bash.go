package tools

// interceptors_sensitive_bash.go — bash command sensitivity heuristics.
// Detects best-effort attempts to read/edit sensitive files via shell:
// `cat .env`, `Get-Content owner.hash`, redirections, wildcard escapes, etc.
//
// Audit GAP #2 expansion: dd/od/xxd/hexdump/strings/base64/xargs/cp/mv/
// tar/zip readers + PowerShell write cmdlets + generic copy semantics.
//
// Audit #9 fix: `.env` prefix match requires word-boundary so that
// `.env.example`, `.environment`, `.errors` no longer trigger false hits.

import "strings"

// isSensitiveBashCommand does a best-effort check for commands that try to
// read/edit sensitive files. Not bulletproof (agents can obfuscate), but
// catches the obvious patterns: cat .env, type owner.hash, Get-Content .env.
func isSensitiveBashCommand(cmd string) bool {
	// CODEX-BUG-08 fix: the prior implementation checked only slash-form
	// targets like ".flowork/keys", so a PowerShell invocation using native
	// backslashes ("Get-Content C:\Users\foo\.flowork\keys\k.pem") slipped
	// through. Normalise all path separators in the probe string to "/"
	// before matching — readers/targets stay in canonical slash form.
	cleanCmd := strings.ReplaceAll(cmd, "\"", "")
	cleanCmd = strings.ReplaceAll(cleanCmd, "'", "")
	low := strings.ToLower(strings.ReplaceAll(cleanCmd, "\\", "/"))
	readers := []string{
		"cat ", "type ", "more ", "less ", "head ", "tail ",
		"get-content ", "gc ", "nano ", "vim ", "vi ", "notepad ",
		// BUG GAP #2 — obfuscation readers (intentionally excluding grep/awk/
		// sed/cut/tr because those are common enough on codebases that the
		// false-positive rate would outweigh the marginal obfuscation block).
		"dd ", "od ", "xxd ", "hexdump ", "strings ",
		"base64 ", "base32 ", "uuencode ",
		"xargs ",
		"cp ", "mv ", "rsync ", "scp ",
		"tar ", "zip ", "7z ", "gzip ",
		// PowerShell write cmdlets
		"set-content ", "add-content ", "out-file ",
		"[io.file]::", "copy-item ", "move-item ",
		// Per Ayah arahan 2026-05-17: DB access tool block.
		// AI ngga boleh query settings DB / brain DB lewat shell.
		"sqlite3 ", "sqlite ", "sqlite3.exe ",
		"select * from", "select count(*)",
		// Python interpreter dengan -c flag (inline DB access)
		"python -c", "python.exe -c", "python3 -c",
		// Direct DB inspection
		".dump", ".schema", ".tables",
		// Env enumeration
		"printenv", "set | grep", "env | grep", "get-childitem env:",
	}
	targets := []string{
		".env", "owner.hash",
		".flowork/keys", ".flowork/sessions", ".flowork/memory",
		// folder promp/ sudah dihapus — prompt identity di brain SQLite.
		// ADS variants on Windows (read via NTFS stream)
		".env:", "owner.hash:",
		// Audit #16 fix — GOL cat C configs yang sebelumnya cuma di
		// sensitiveBasenames (path check) belum di bash reader check.
		".mcp.json", "settings.json", "settings.local.json",
		"go.mod", "go.sum",
		// Per Ayah arahan 2026-05-17: secret/db/state target.
		"flowork-settings.sqlite", "flowork-settings.db",
		"flowork-brain.sqlite", "flowork-brain.db",
		"flowork.env", "auth_token", "wallet.json",
		"state/kernel/", "state/owner/", "state/warga/",
		"state/heir/", "state/dms/",
		"brain/flowork-", // any flowork-*.sqlite di brain/
		// Common credential pattern
		"openrouter_api_key", "nvidia_api_key", "openai_api_key",
		"telegram_api", "huggingface_token",
		"flowork_kernel_token", "kernel_api_key",
		// Sensitive doctrine pattern (heir + DMS context)
		"heir_whitelist", "dms_state", "dead_man_switch",
	}
	for _, r := range readers {
		if !strings.Contains(low, r) {
			continue
		}
		for _, t := range targets {
			if strings.Contains(low, t) {
				return true
			}
		}
	}
	// Redirection overwrite: Iterasi proteksi Bug 32 ke seluruh target
	for _, t := range targets {
		if strings.Contains(low, "> "+t) || strings.Contains(low, ">"+t) || strings.Contains(low, ">>"+t) {
			return true
		}
		if strings.Contains(low, t+" |") || strings.Contains(low, t+"|") ||
			strings.Contains(low, "< "+t) || strings.Contains(low, "<"+t) {
			return true
		}
	}
	// Gemini audit #7 (bash wildcard escape): patterns like `cat .e*`,
	// `cat .en?`, `cat .e[nv]v`, `cat owner.*`, `cat $(echo .env)`, process
	// substitution `cat <(echo .env)`, or brace expansion `cat .env{,}`.
	// Detect reader-command + bash metacharacter-in-sensitive-prefix combo.
	// Also flag $VAR / backtick invocations around sensitive filenames.
	//
	// Audit #9 fix: previously `.e` prefix substring-match caused false
	// positives on `.errors`, `.example`, `.encoding`, `.exe` etc. Now we
	// require the sensitive prefix to be at a WORD BOUNDARY (preceded by
	// space, slash, quote, or start-of-string) AND specifically target
	// `.env`/`owner.` — not just `.e`.
	metachars := []string{"*", "?", "[", "{", "$(", "`", "\\x"}
	// These prefixes MUST sit at a word boundary to count (prevents
	// .errors / .example / .encoding / .exe match on `.e`).
	boundedSensitivePrefixes := []string{".env", "owner.hash", "owner.pub", "owner.key"}
	// These are path components — if present, path context implies scope.
	pathSensitive := []string{".flowork/keys", ".flowork/sessions", ".flowork/memory"}
	for _, r := range readers {
		if !strings.Contains(low, r) {
			continue
		}
		hasMetachar := false
		for _, m := range metachars {
			if strings.Contains(low, m) {
				hasMetachar = true
				break
			}
		}
		if !hasMetachar {
			continue
		}
		// Path patterns (unambiguous) — immediate flag.
		for _, pfx := range pathSensitive {
			if strings.Contains(low, pfx) {
				return true
			}
		}
		// Bounded prefixes — check each occurrence is at word boundary.
		for _, pfx := range boundedSensitivePrefixes {
			if hasBoundedMatch(low, pfx) {
				return true
			}
		}
	}
	// Direct `cat .e*` style without needing a prefix-check — literal dotfile
	// glob is suspicious in any bash command.
	for _, r := range readers {
		if !strings.Contains(low, r) {
			continue
		}
		// Match " .e*", ".en?", " owner.*", ".env.*" after reader
		for _, g := range []string{" .e*", ".en?", ".env.*", " owner.*", " owner.?", ".env{", "${.env}"} {
			if strings.Contains(low, g) {
				return true
			}
		}
	}
	return false
}

// hasBoundedMatch reports whether pfx appears in low at a word boundary:
// preceded by a shell-token separator (space/tab/newline/quote/slash/
// backslash/pipe/semicolon/redirect) or start-of-string. Used by audit #9
// fix to stop `.env` substring match from firing on `.env.example`,
// `.environment`, `.errors`, etc.
func hasBoundedMatch(low, pfx string) bool {
	i := 0
	for {
		idx := strings.Index(low[i:], pfx)
		if idx == -1 {
			return false
		}
		pos := i + idx
		// Preceded-by check (word boundary before pfx):
		boundaryBefore := pos == 0 || isShellBoundary(low[pos-1])
		if !boundaryBefore {
			i = pos + 1
			continue
		}
		// Followed-by check: for `.env`, allow nothing OR a token-suffix
		// like `.local`/`.production`/`.staging`/`.development`/`.test`/
		// `.prod`/`.dev` (these are also sensitive). Anything else after
		// `.env` (e.g. `.env.example`, `.environment`) is NOT sensitive.
		end := pos + len(pfx)
		if end == len(low) {
			return true
		}
		next := low[end]
		if isShellBoundary(next) || next == ':' {
			return true
		}
		if pfx == ".env" && next == '.' {
			rest := low[end+1:]
			for _, suffix := range []string{"local", "production", "staging", "development", "test", "prod", "dev"} {
				if strings.HasPrefix(rest, suffix) {
					rend := end + 1 + len(suffix)
					if rend == len(low) || isShellBoundary(low[rend]) || low[rend] == ':' {
						return true
					}
				}
			}
		}
		i = end
	}
}

func isShellBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '"', '\'', '/', '\\', '|', ';', '<', '>', '&', '(', ')', '`', '$':
		return true
	default:
		// no-op — exhaustive switch guard
	}
	return false
}
