// Package guardian — ROADMAP Guardian FASE 1 (L2): Boot Integrity Gate + SAFE-MODE.
//
// Setelah Kernel FREEZE, guardian memastikan binary + file inti TIDAK berubah diam-diam.
// Akar kepercayaan = immutability OS + hak owner (BUKAN crypto/signature — sesuai filosofi).
// FASE 1 ini murni DETEKSI: lintas-OS, NO ROOT, hash-compare vs baseline di vault.
//
// Alur: owner `--arm` → rekam baseline (hash binary + file inti) ke vault, armed=true.
// Boot berikutnya (armed) → re-hash → beda? → SAFE-MODE (neuter exec/install via middleware)
// + alert owner. Guardian TIDAK menyentuh kernel beku — enforcement di middleware luar (main.go).
//
// FASE 2 (nyusul) = OS-immutability adapter (Sealer per-OS). FASE 3 = sentinel runtime.
package guardian

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
)

// selfKey — kunci baseline khusus untuk binary yang sedang jalan.
const selfKey = "@self"

// Vault — state guardian, satu sumber kebenaran (~/.flowork/guardian/vault.json).
// Di FASE 2 file ini yang disegel OS supaya baseline tak bisa dipalsu.
type Vault struct {
	Armed    bool              `json:"armed"`
	Mode     string            `json:"mode"`     // "safe" (default) — respons tamper
	Baseline map[string]string `json:"baseline"` // key -> sha256 hex ("@self" = binary)
	SealedAt string            `json:"sealed_at"`
}

// safeMode — flag global runtime (di-set saat boot kalau integritas gagal).
var safeMode atomic.Bool

// SafeMode — true kalau guardian mendeteksi tamper (neuter mode aktif).
func SafeMode() bool { return safeMode.Load() }

// EnterSafeMode — aktifkan neuter mode (dipanggil boot gate saat mismatch).
func EnterSafeMode() { safeMode.Store(true) }

// VaultPath — ~/.flowork/guardian/vault.json.
func VaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".flowork", "guardian", "vault.json")
}

// Load — baca vault. Kalau belum ada → Vault kosong (armed=false, guardian pasif/dev mode).
func Load() (*Vault, error) {
	v := &Vault{Mode: "safe", Baseline: map[string]string{}}
	raw, err := os.ReadFile(VaultPath())
	if err != nil {
		if os.IsNotExist(err) {
			return v, nil
		}
		return v, err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return v, err
	}
	if v.Baseline == nil {
		v.Baseline = map[string]string{}
	}
	if v.Mode == "" {
		v.Mode = "safe"
	}
	return v, nil
}

// Save — tulis vault (atomic via temp+rename).
func (v *Vault) Save() error {
	p := VaultPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Verify — re-hash tiap entri baseline, balik (cocok?, daftar masalah). Dipanggil boot gate.
func (v *Vault) Verify() (bool, []string) {
	var problems []string
	for key, want := range v.Baseline {
		var got string
		var err error
		if key == selfKey {
			got, err = selfHash()
		} else {
			got, err = hashFile(key)
		}
		if err != nil {
			problems = append(problems, "hilang/tak terbaca: "+key)
			continue
		}
		if got != want {
			problems = append(problems, "berubah: "+key)
		}
	}
	return len(problems) == 0, problems
}

// Arm — rekam baseline (binary + file inti yang ADA) → armed=true. now = timestamp dari caller
// (package ini tak panggil time.Now sendiri biar deterministik/testable).
func Arm(coreFiles []string, now string) (*Vault, error) {
	v, err := Load()
	if err != nil {
		return nil, err
	}
	base := map[string]string{}
	if h, herr := selfHash(); herr == nil {
		base[selfKey] = h
	}
	for _, f := range coreFiles {
		if h, herr := hashFile(f); herr == nil {
			base[f] = h
		}
	}
	v.Armed = true
	if v.Mode == "" {
		v.Mode = "safe"
	}
	v.Baseline = base
	v.SealedAt = now
	if err := v.Save(); err != nil {
		return nil, err
	}
	return v, nil
}

// Disarm — matikan guardian (armed=false). Buat update kernel/binary yang disengaja owner.
func Disarm() error {
	v, err := Load()
	if err != nil {
		return err
	}
	v.Armed = false
	return v.Save()
}

// selfHash — sha256 dari binary yang sedang jalan.
func selfHash() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	return hashFile(exe)
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

var manifestPathRe = regexp.MustCompile(`(internal/[^\s]+\.go)`)

// CoreFilesFromManifest — baca daftar file inti dari KERNEL_FREEZE.md (kalau ada di cwd).
// Deploy binary-only (tanpa source) → balik kosong → guardian cuma jaga @self (binary).
func CoreFilesFromManifest() []string {
	f, err := os.Open("KERNEL_FREEZE.md")
	if err != nil {
		return nil
	}
	defer f.Close()
	seen := map[string]bool{}
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := manifestPathRe.FindString(sc.Text()); m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// ── SAFE-MODE enforcement (middleware luar — TIDAK menyentuh kernel beku) ──────

// dangerousPrefixes — endpoint berbahaya yang diblok saat SAFE-MODE (exec/install/escalate).
// Endpoint baca (GET status/chat/login/audit) TETAP jalan → owner bisa investigasi.
var dangerousPrefixes = []string{
	"/api/agents/tools/run",
	"/api/plugins/install",
	"/api/coder/",
	"/api/scanner/run",
	"/api/scanner/distill",
	"/api/scanner/bodyscan",
	"/api/apps/install",
	"/api/tools/install",
	"/api/slash/install",
	"/api/mcp/install",
	"/api/kernel/rpc",
	"/api/kernel/call",
}

// IsDangerousPath — true kalau path = permukaan exec/install yang harus diblok di SAFE-MODE.
func IsDangerousPath(p string) bool {
	for _, pre := range dangerousPrefixes {
		if p == pre || strings.HasPrefix(p, pre) {
			return true
		}
	}
	return false
}

// SafeModeMiddleware — lapis paling luar. Kalau SAFE-MODE aktif, endpoint berbahaya → 503.
// Wiring di main.go (file TIDAK beku). Nol perubahan ke kernel.
func SafeModeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SafeMode() && IsDangerousPath(r.URL.Path) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"guardian-safe-mode: integritas kernel gagal — exec/install diblok. Cek alert + disarm setelah investigasi."}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
