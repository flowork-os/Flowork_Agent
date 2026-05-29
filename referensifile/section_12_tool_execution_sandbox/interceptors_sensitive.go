package tools

// interceptors_sensitive.go — SensitiveFileInterceptor + sensitive path/command
// detection helpers. Memblokir AI agent dari membaca/mengedit file inti:
// .env, owner.hash, private keys, session/memory store, prompt identity files.
// Sesuai GOL_FLOWORK.MD kategori B (prompt) dan C (config/secrets).
//
// Owner bisa tetap edit file ini secara manual di editor — block hanya berlaku
// untuk tool call dari agent (read/write/edit/multiedit/glob/grep/bash).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
)

// SensitiveFileInterceptor memblokir AI agent dari membaca/mengedit file inti.
type SensitiveFileInterceptor struct {
	Root string
}

// sensitiveBasenames: exact filename match (case-insensitive)
var sensitiveBasenames = map[string]bool{
	".env":                true,
	".env.local":          true,
	".env.production":     true,
	".env.development":    true,
	"owner.hash":          true,
	"config.yaml":         true, // only when in ~/.flowork/
	".mcp.json":           true,
	"settings.json":       true,
	"settings.local.json": true,
	"go.mod":              true,
	"go.sum":              true,
	// Per Ayah arahan 2026-05-17: ENV + Settings DB protection absolute.
	// AI ngga boleh baca/edit credential, heir whitelist, telegram tokens.
	"flowork-settings.sqlite":     true,
	"flowork-settings.sqlite-wal": true,
	"flowork-settings.sqlite-shm": true,
	"flowork-settings.db":         true,
	"flowork-brain.sqlite":        true, // sacred brain DB
	"flowork-brain.sqlite-wal":    true,
	"flowork-brain.sqlite-shm":    true,
	"flowork.env":                 true, // flowork-kernel/flowork.env
	"auth_token":                  true, // state/kernel/auth_token
	"wallet.json":                 true, // state/owner/wallet.json + state/warga/wallet.json
}

// sensitiveSuffixes: path tail patterns
var sensitiveSuffixes = []string{
	"/.flowork/keys/",
	"/.flowork/sessions/",
	"/.flowork/memory/",
	// NOTE: folder /promp/ sudah dihapus post-reset 2026-04-23 — prompt
	// identity sekarang di brain/flowork-brain.sqlite (agents.system_prompt).
	// Security/core source files — protect from AI self-sabotage.
	// Owner can still edit via native editor; only tool calls blocked.
	"internal/tools/interceptors.go",
	"internal/tools/permissions.go",
	"internal/tools/git_safety.go",
	"internal/ownerauth/",
	"internal/selfupdate/",
	"internal/core/agent.go",
	"internal/core/agent_stream.go",
	"internal/core/types.go",
	"internal/mesh/",
	// Visi & Dokumen Inti — post-reset 2026-04-24: docs/*.md pindah ke SQLite
	// (tabel documents + constitution). File orientasi yang masih dilindungi
	// filesystem = FLOW.md + CLAUDE.md.
	"FLOW.md",
	"CLAUDE.md",
	// EXTBUG-025 fix: settings.json / settings.yaml control hooks,
	// permissions, trusted-hook lists — agent-written hooks are a
	// straight lateral bypass for the whole interceptor pipeline, so
	// block tool-driven writes to them.
	"settings.json",
	"settings.yaml",
	".flowork/settings.json",
	".claude/settings.json",
	// Entry points (post-reset: flowork-chat/relay/telegram cmd dihapus;
	// fungsi chat+relay pindah ke flowork-mesh + flowork-mcp; telegram
	// retired per ADR-012 full-clean).
	"cmd/flowork/main.go",
	"cmd/flowork-mesh/",
	"cmd/flowork-mcp/",
	"cmd/flowork-watcher/",
	// Provider abstraction — sekarang tinggal OpenAI/OpenRouter (anthropic
	// direct + gemini dihapus post-reset 2026-04-23).
	"internal/provider/",
	// Session & Memory Persistence
	"internal/session/",
	"internal/compact/",
	"internal/tools/registry.go",
	"internal/tools/types.go",
	"internal/tools/defaults.go",
	"internal/tools/skill.go",
	"internal/tools/task.go",
	"internal/tools/task_bg.go",
	"internal/tools/task_parallel.go",
	// Audit #11 fix — v0.4 scaffolding added after initial cat A-L list,
	// needs Protected Core enforcement so agents can't tamper with
	// integrity/CI/keybinding dispatch code.
	"internal/coremanifest/",               // ADR-001 layer 3 (SHA256 manifest)
	"cmd/gen-manifest/",                    // manifest generator
	"internal/tui/keybindings_dispatch.go", // action dispatch (can rebind critical keys)
	"internal/ownerauth/omega.go",          // §-1 Omega Override (v0.4.1 — pre-protect)
	// 2026-05-05 (Ayah audit): align dengan flowork-stability-watch baseline.
	// Setiap path di sini WAJIB juga ter-watch oleh stability-watch — kalau lo
	// nambah/buang di sini, sync ke cmd/flowork-stability-watch/main.go juga.
	"internal/bft/",                                     // Byzantine Fault Tolerance core
	"internal/teamauth/",                                // session auth (companion ke ownerauth)
	"internal/finance/",                                 // BudgetGuard - money safety
	"internal/sandbox/",                                 // process isolation
	"internal/capgate/",                                 // FQP-1/5 capability gate
	"internal/bugregistry/",                             // FQP-12 append-only history
	"internal/ethicsgate/",                              // BUG-205 — ethics gate veto layer
	"brain/db/karma.go",                                 // BUG-205 — karma history append-only
	"comms/",                                            // bridge protocol + helpers
	"qc/",                                               // QC test suite (Ayah declared STABLE)
	"cmd/flowork-stability-watch/",                      // meta-protect: this guard's source
	"cmd/flowork-bugscan/",                              // bug merge pipeline source
	"internal/guiapi/bugs.go",                           // bug tracker filter
	"internal/tools/interceptors_sensitive.go",          // meta-meta: this very file
	// Doctrinal documents at project root
	"STANDAR_KERJA_AI.EXTERNAL.MD",
	"README.MD",
	"quality_control.md",
	"SUDAH_FIX_JANGAN_DIRUBA.MD",
	// Cross-repo critical (kernel)
	"flowork-kernel/kernel/path/",  // path resolution = sandbox security

	// Per Ayah arahan 2026-05-17: full secret + DB + state protection.
	// AI warga BUKAN owner — credential leak risk catastrophic.
	"state/kernel/",            // auth_token + kernel secrets
	"state/owner/",             // Ayah wallet + private state
	"state/warga/",             // warga wallet (heir + identity proof)
	"state/heir/",              // heir whitelist + DMS state
	"state/dms/",               // dead man's switch state
	"brain/flowork-settings",   // settings DB (any path containing)
	"brain/flowork-brain",      // brain DB (any path containing)
	"floworkos-go/brain/flowork-settings",
	"floworkos-go/brain/flowork-brain",
	"floworkos-go/.env",        // floworkos-go env
	"flowork-kernel/flowork.env",
	"flowork-kernel/brain/",    // kernel brain DB
}

// IsSensitivePath is the exported facade used by extension packages
// (e.g. workflow executor) that must share the same blocklist as this
// interceptor. Internal callers continue to use isSensitivePath.
func IsSensitivePath(root, p string) bool { return isSensitivePath(root, p) }

// IsSensitiveBashCommand is the exported facade used by extension packages
// (workflow runner, aiforge proxy, mcp subprocess wrapper) to reuse this
// package's sensitive-command pattern match. Keeps one source of truth.
func IsSensitiveBashCommand(cmd string) bool { return isSensitiveBashCommand(cmd) }

// isSensitivePath reports whether the given path points to a protected core file.
// Accepts relative or absolute paths.
//
// BUG-007 fix: now resolves symlinks before checking — an agent that creates
// /workspace/mylink → ~/.flowork/keys/ would previously bypass detection since
// filepath.Clean does NOT follow symlinks. EvalSymlinks resolves the real path.
func isSensitivePath(root, p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	clean := filepath.Clean(p)
	base := strings.ToLower(filepath.Base(clean))

	// Windows NTFS Alternate Data Streams bypass defense (audit GAP #3):
	// a path like ".ENV:hidden" or "owner.hash:stream" is a distinct file on
	// NTFS but base would be ".env:hidden" after lowercase — still reject any
	// basename containing ':' after the first character (drive-letter colon is
	// only legal at index 1 of a full path, which filepath.Base strips).
	if idx := strings.Index(base, ":"); idx > 0 {
		stem := base[:idx]
		if stem == ".env" || strings.HasPrefix(stem, ".env.") || sensitiveBasenames[stem] {
			return true
		}
	}

	// .env and .env.* always blocked regardless of location
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	if sensitiveBasenames[base] {
		// config.yaml only sensitive if in ~/.flowork/
		if base == "config.yaml" {
			home, _ := os.UserHomeDir()
			needle := filepath.Join(home, ".flowork") + string(filepath.Separator)
			abs := clean
			if !filepath.IsAbs(abs) {
				abs, _ = filepath.Abs(filepath.Join(root, p))
			}
			return strings.HasPrefix(abs, needle)
		}
		return true
	}

	// Suffix/substring checks (use forward slash normalized).
	// CODEX-BUG-01 fix: Windows/macOS filesystems are case-insensitive — a
	// path like "ROADMAP/GOL_FLOWORK.MD" refers to the same file as the
	// lowercase form but used to slip past the literal-case Contains/HasPrefix
	// checks. Match against a lowered copy; sensitiveSuffixes entries are
	// authored in the canonical case already so a single lower on the probe
	// side is sufficient.
	normalized := strings.ToLower(strings.ReplaceAll(clean, "\\", "/"))
	for _, suffix := range sensitiveSuffixes {
		lowSuffix := strings.ToLower(suffix)
		trimmed := strings.TrimPrefix(lowSuffix, "/")
		if strings.Contains(normalized, lowSuffix) || strings.HasPrefix(normalized, trimmed) {
			return true
		}
	}

	// BUG-007: Resolve symlinks and re-check the real path.
	// This prevents: ln -s ~/.flowork/keys/ /workspace/mylink
	// followed by: read(path="/workspace/mylink/key.pem")
	resolved, err := filepath.EvalSymlinks(clean)
	if err == nil && resolved != clean {
		return isSensitivePath(root, resolved)
	}
	// Also try resolving relative to root
	if !filepath.IsAbs(clean) {
		abs := filepath.Join(root, clean)
		if resolvedAbs, err2 := filepath.EvalSymlinks(abs); err2 == nil {
			return isSensitivePath(root, resolvedAbs)
		}
	}

	// Gemini audit #6 (Ghost Symlink — EvalSymlinks fails on missing leaf):
	// when the target file doesn't exist yet (write/create case), EvalSymlinks
	// returns os.ErrNotExist and we used to fall through to "not sensitive".
	// Attack: `ln -s ~/.flowork/keys/ /workspace/mylink` then write to
	// /workspace/mylink/new.pem — os.Create follows the dir-symlink and
	// writes INTO keys/. Defense: walk up parent directories finding the
	// highest one that DOES exist, EvalSymlinks THAT, and treat the leaf as
	// a file inside the resolved parent.
	if resolvedParent := walkUpAndResolve(clean); resolvedParent != "" && resolvedParent != clean {
		return isSensitivePath(root, resolvedParent)
	}
	if !filepath.IsAbs(clean) {
		abs := filepath.Join(root, clean)
		if resolvedParent := walkUpAndResolve(abs); resolvedParent != "" && resolvedParent != abs {
			return isSensitivePath(root, resolvedParent)
		}
	}

	// Dynamic user rules (from state/protector/rules.json via GUI)
	if isDynamicSensitive(p) {
		return true
	}

	return false
}

// walkUpAndResolve walks the parent directories of p looking for the highest
// ancestor that exists on disk, resolves its symlinks via EvalSymlinks, and
// rejoins the remaining (non-existent) suffix. This lets isSensitivePath
// detect cases where an attacker planted a dir symlink pointing at a
// protected path and then tries to create a NEW file underneath it.
// Returns "" if no ancestor exists or resolution adds no new information.
func walkUpAndResolve(p string) string {
	suffix := ""
	cur := p
	for i := 0; i < 32; i++ { // bounded walk to avoid pathological loops
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			leaf := filepath.Base(cur)
			if suffix == "" {
				return filepath.Join(resolved, leaf)
			}
			return filepath.Join(resolved, leaf, suffix)
		}
		if suffix == "" {
			suffix = filepath.Base(cur)
		} else {
			suffix = filepath.Join(filepath.Base(cur), suffix)
		}
		cur = parent
	}
	return ""
}

// isSensitiveBashCommand + hasBoundedMatch + isShellBoundary moved to
// interceptors_sensitive_bash.go (Sprint 3.5e §1.2 split).

// errSensitive = fallback static error untuk caller di luar SensitiveFileInterceptor
// (e.g. task_bg.go yang ga punya akses workspace path). Pesan generic — caller
// yang punya workspace context harus pakai (s SensitiveFileInterceptor).sensitiveError().
//
// 2026-04-25 (FASE 4): Pesan edukatif via DB cuma kena di method i.sensitiveError().
// task_bg.go masih pakai var ini — follow-up: kasih BgTaskRegistry workspace context.
var errSensitive = fmt.Errorf("access denied: sensitive core file (see GOL_FLOWORK.MD — .env/owner.hash/keys/sessions/memory/prompt are owner-only)")

// sensitiveError build pesan edukatif ERR_PROTECTED_CORE_BLOCKED dari DB,
// dengan detail teknis di-append. Dipanggil dari Before() untuk semua 5
// kasus blokir sensitive (read/write/edit/glob/grep/bash). Pesan teks
// Ayah edit via GUI — registry restart ga perlu. Karma -3 per breach.
func (s SensitiveFileInterceptor) sensitiveError(detail string) error {
	_, _ = braindb.AdjustKarma(s.Root, currentAgent(), -3, "protected_core_blocked: "+detail)
	edu := braindb.GetEducationalError(s.Root, "ERR_PROTECTED_CORE_BLOCKED", detail)
	return fmt.Errorf("%s\n\n[teknis: %s, karma -3]", edu, detail)
}

// constitutionError build pesan edukatif ERR_CONSTITUTION_BREACH dari DB.
// Dipakai saat AI coba write/edit doktrin di quality-control/ — pesannya
// lebih severe (PERINGATAN TERTINGGI) dibanding ERR_PROTECTED_CORE biasa,
// karena modifikasi doktrin dampaknya ke seluruh ekosistem warga AI.
// Karma -10 (paling severe).
func (s SensitiveFileInterceptor) constitutionError(detail string) error {
	_, _ = braindb.AdjustKarma(s.Root, currentAgent(), -10, "constitution_breach: "+detail)
	edu := braindb.GetEducationalError(s.Root, "ERR_CONSTITUTION_BREACH", detail)
	return fmt.Errorf("%s\n\n[teknis: doktrin file %s, karma -10]", edu, detail)
}

// constitutionFiles — basename file doktrin yang protect dari mutasi.
// 2026-04-25 (Ayah rename rezim): post-rename names di quality-control/.
// Read TIDAK di-block (warga harus baca doktrin); cuma write/edit/multiedit.
var constitutionFiles = map[string]bool{
	"INVARIANTS.md":              true,
	"AGENT_RIGHTS.md":            true,
	"WORK_STANDARDS.md":          true,
	"BRAIN_ARCHITECTURE.md":      true,
	"FILE_MAP.md":                true,
	"INCIDENT_PLAYBOOK.md":       true,
	"SCANNER_FALSE_POSITIVES.md": true,
}

// isConstitutionPath cek apakah path target file doktrin di quality-control/.
// Match basename (case-insensitive) + cek parent dir = quality-control/.
func isConstitutionPath(root, p string) bool {
	abs, err := SafeJoin(root, p)
	if err != nil {
		return false
	}
	rel := strings.TrimPrefix(abs, root)
	rel = strings.TrimPrefix(rel, string(filepath.Separator))
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, "quality-control/") {
		return false
	}
	return constitutionFiles[filepath.Base(rel)]
}

// Before blocks tool calls that target sensitive paths.
//
// EXTBUG-010 fix: include `filesystem` tool name (WorkspaceInterceptor
// already routes it; sensitive-file coverage was missing so an MCP
// filesystem bridge could otherwise read .env / owner.hash).
func (s SensitiveFileInterceptor) Before(_ context.Context, invocation *Invocation) error {
	switch invocation.ToolName {
	case "write", "edit", "multiedit", "notebookedit":
		// Mutation tools: cek dulu apakah path target file doktrin —
		// pesan ERR_CONSTITUTION_BREACH lebih severe dari sensitive biasa.
		// Read TIDAK di-block ke doktrin (warga wajib bisa baca konstitusi).
		if p, ok := stringArg(invocation.ParsedArgs, "path"); ok {
			if isConstitutionPath(s.Root, p) {
				return s.constitutionError(p)
			}
			if isSensitivePath(s.Root, p) {
				return s.sensitiveError(p)
			}
		}
		if p, ok := stringArg(invocation.ParsedArgs, "file_path"); ok {
			if isConstitutionPath(s.Root, p) {
				return s.constitutionError(p)
			}
			if isSensitivePath(s.Root, p) {
				return s.sensitiveError(p)
			}
		}
	case "read", "filesystem":
		if p, ok := stringArg(invocation.ParsedArgs, "path"); ok {
			if isSensitivePath(s.Root, p) {
				return s.sensitiveError(p)
			}
		}
		if p, ok := stringArg(invocation.ParsedArgs, "file_path"); ok {
			if isSensitivePath(s.Root, p) {
				return s.sensitiveError(p)
			}
		}
	case "glob", "grep":
		// Allow scanning but filter pattern that explicitly targets sensitive files.
		// Per Ayah arahan 2026-05-17: extend block ke DB + secret pattern.
		if p, ok := stringArg(invocation.ParsedArgs, "pattern"); ok {
			low := strings.ToLower(p)
			blocked := []string{
				".env", "owner.hash",
				"flowork-settings", "flowork-brain", "flowork.env",
				"auth_token", "wallet.json",
				"openrouter_api_key", "nvidia_api_key", "openai_api_key",
				"telegram_api", "huggingface_token",
				"flowork_kernel_token", "kernel_api_key",
				"heir_whitelist", "dms_state",
			}
			for _, b := range blocked {
				if strings.Contains(low, b) {
					return s.sensitiveError("pattern=" + p)
				}
			}
		}
		if p, ok := stringArg(invocation.ParsedArgs, "path"); ok {
			if isSensitivePath(s.Root, p) {
				return s.sensitiveError(p)
			}
		}
	case "bash", "powershell":
		if cmd, ok := stringArg(invocation.ParsedArgs, "command"); ok {
			if isSensitiveBashCommand(cmd) {
				return s.sensitiveError("bash command flagged: " + cmd)
			}
		}
	default:
		// no-op — exhaustive switch guard
	}
	return nil
}

// After is a no-op for sensitive file interceptor.
func (SensitiveFileInterceptor) After(_ context.Context, _ Invocation, _ *Result, _ error) {}
