// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval. (LOCKED ≠ FREEZE.)
// Owner: Aola Sahidin (Mr.Dev) · Locked at: 2026-06-16. Reason: R7 B3 auto-push + rollback-log.
// VERIFIED E2E: push-config set/get (token MASKED, ga pernah dibalikin); auto-commit→push ke
// local remote berhasil (b0b37b7). Token di KV (~/.flowork, ga ke-commit); push via http.extraHeader
// (token ga masuk .git/config/URL); error push disanitasi. Gated: enabled flag + token + jalur auto.
//
// selfevolve_push.go — R7 fase-2b B3: AUTO-PUSH (organisme self-publish biar abadi).
// Owner-approved 2026-06-16: visi organisme terus hidup & nyebarin evolusi sendiri bahkan
// setelah owner tiada → butuh token push SENDIRI (bukan token owner). Token disimpan di KV
// flowork.db (di ~/.flowork, DI LUAR git → ga pernah ke-commit). Setting page buat ngisi.
//
// Push pakai http.extraHeader (token TIDAK tersimpan ke .git/config, TIDAK masuk URL). GET
// config gak pernah balikin token (cuma has_token bool). Gated: enabled flag + token ada —
// DAN cuma dipanggil dari jalur auto-commit yg udah lolos SEMUA gate (dev+auto+karma+model+
// re-probe). Lapisan ekstra "enabled" = saklar khusus self-push (kepisah dari mode=auto).

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flowork-gui/internal/floworkdb"
)

// evolveDataDir — data dir portable (FLOWORK_DATA_DIR > ~/.flowork), mirror floworkdb +
// evolve-rollback.sh. No-hardcode lokasi.
func evolveDataDir() string {
	if d := strings.TrimSpace(os.Getenv("FLOWORK_DATA_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".flowork")
}

// evolveRollbackLogHandler — GET /api/evolve/rollback-log → isi evolve-rollback.log. Penyebab
// organisme hampir mati: commit letal (build router/agent rusak) yg di-revert watchdog (organ
// independen). Biar organisme TAU PENYEBAB-nya pas hidup lagi (ide owner). Read-only.
func evolveRollbackLogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir := evolveDataDir()
		if dir == "" {
			tfWriteJSON(w, 0, map[string]any{"log": "", "note": "data dir ga ketemu"})
			return
		}
		b, err := os.ReadFile(filepath.Join(dir, "evolve-rollback.log"))
		if err != nil {
			tfWriteJSON(w, 0, map[string]any{"log": "", "note": "belum ada rollback — organisme belum pernah hampir mati dari commit letal"})
			return
		}
		s := string(b)
		if len(s) > 20000 {
			s = "…(dipotong)…\n" + s[len(s)-20000:]
		}
		tfWriteJSON(w, 0, map[string]any{"log": s})
	}
}

type evolvePushCfg struct {
	Enabled bool
	Remote  string
	Token   string
	Branch  string
}

// evolveLoadPushCfg — baca config push dari KV (lokal, ga ke-commit). Default remote=origin.
func evolveLoadPushCfg() evolvePushCfg {
	cfg := evolvePushCfg{Remote: "origin"}
	db, err := floworkdb.Shared()
	if err != nil {
		return cfg
	}
	if v, _ := db.GetKV("evolve_push_enabled"); v == "1" {
		cfg.Enabled = true
	}
	if v, _ := db.GetKV("evolve_push_remote"); strings.TrimSpace(v) != "" {
		cfg.Remote = strings.TrimSpace(v)
	}
	// Token = KREDENSIAL → tabel `secrets` (enkripsi-at-rest), BUKAN kv. Migrasi otomatis
	// dari kv lama (kalau ada) → secrets sekali, terus bersihin kv (owner 2026-06-20 fix kritis).
	cfg.Token, _ = db.GetSecret("evolve_push_token")
	if cfg.Token == "" {
		if legacy, _ := db.GetKV("evolve_push_token"); strings.TrimSpace(legacy) != "" {
			_ = db.SetSecret("evolve_push_token", legacy)
			_ = db.SetKV("evolve_push_token", "") // cabut dari kv (plaintext) setelah pindah
			cfg.Token = legacy
		}
	}
	cfg.Branch, _ = db.GetKV("evolve_push_branch")
	return cfg
}

// evolvePushConfigHandler — GET status (token MASKED) / POST set {enabled,remote,token,branch}.
// Owner-loopback. Token cuma ditulis, GA PERNAH dibalikin (cuma has_token bool).
func evolvePushConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, err := floworkdb.Shared()
		if err != nil {
			tfWriteJSON(w, 0, map[string]any{"error": "db: " + err.Error()})
			return
		}
		if r.Method == http.MethodPost {
			var b struct {
				Enabled *bool   `json:"enabled"`
				Remote  *string `json:"remote"`
				Token   *string `json:"token"`
				Branch  *string `json:"branch"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b.Enabled != nil {
				v := "0"
				if *b.Enabled {
					v = "1"
				}
				_ = db.SetKV("evolve_push_enabled", v)
			}
			if b.Remote != nil {
				_ = db.SetKV("evolve_push_remote", strings.TrimSpace(*b.Remote))
			}
			if b.Branch != nil {
				_ = db.SetKV("evolve_push_branch", strings.TrimSpace(*b.Branch))
			}
			if b.Token != nil {
				// KREDENSIAL → secrets (enkripsi-at-rest), bukan kv. "" = owner cabut token.
				_ = db.SetSecret("evolve_push_token", strings.TrimSpace(*b.Token))
				_ = db.SetKV("evolve_push_token", "") // pastiin ga ada sisa plaintext di kv
			}
			tfWriteJSON(w, 0, map[string]any{"ok": true})
			return
		}
		cfg := evolveLoadPushCfg()
		tfWriteJSON(w, 0, map[string]any{
			"enabled":   cfg.Enabled,
			"remote":    cfg.Remote,
			"branch":    cfg.Branch,
			"has_token": strings.TrimSpace(cfg.Token) != "",
			"note":      "Token GitHub disimpan lokal (~/.flowork, ga ke-commit). Auto-push cuma jalan kalau enabled + token ada + lolos semua gate auto.",
		})
	}
}

// evolveCurrentBranch — branch aktif repo (buat target push default).
func evolveCurrentBranch(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "main"
	}
	b := strings.TrimSpace(string(out))
	if b == "" || b == "HEAD" {
		return "main"
	}
	return b
}

// evolveMaybePush — kalau push enabled + token ada → push HEAD ke remote/branch via token
// header. Balikin (pushed, note). GAK PERNAH balikin/log token. Dipanggil HANYA dari jalur
// auto-commit (udah lolos semua gate). Gagal push ≠ fatal (commit lokal tetep ada).
func evolveMaybePush(ctx context.Context, root string) (bool, string) {
	cfg := evolveLoadPushCfg()
	if !cfg.Enabled {
		return false, "auto-push OFF (commit lokal aja) — aktifin di Setting + isi token buat self-publish"
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return false, "auto-push enabled tapi token kosong — isi token di Setting"
	}
	branch := strings.TrimSpace(cfg.Branch)
	if branch == "" {
		branch = evolveCurrentBranch(root)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + cfg.Token))
	cmd := exec.CommandContext(ctx, "git", "-C", root,
		"-c", "http.extraHeader=Authorization: Basic "+b64,
		"push", cfg.Remote, "HEAD:"+branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Sanitasi: jangan ikutin output yg mungkin ada kredensial; ringkas aja.
		return false, "push gagal (commit lokal aman): " + evolveSanitizePushErr(string(out))
	}
	return true, "ter-push ke " + cfg.Remote + "/" + branch
}

// evolveSanitizePushErr — ringkas + buang baris yg mungkin bocorin URL bertoken.
func evolveSanitizePushErr(s string) string {
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.Contains(ln, "x-access-token") || strings.Contains(ln, "Authorization") || strings.Contains(ln, "https://") {
			continue
		}
		if t := strings.TrimSpace(ln); t != "" {
			keep = append(keep, t)
		}
	}
	return trimStr(strings.Join(keep, "; "), 200)
}
