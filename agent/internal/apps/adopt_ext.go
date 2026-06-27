// adopt_ext.go — SIBLING package apps (NON-frozen, seam). Jantung "repo → app"
// (ROADMAP_REPO_TO_APP F3): clone/copy repo → deteksi runtime → install dep ke folder →
// generate manifest.json + adapter.json → reloadOne → app LIVE. NOL edit apps.go (substrat utuh).
//
// Repo mentah ga usah ngerti protokol Flowork: core_entry app = binary fw-app-adapter (cliadapter),
// yang nerjemahin op "run" jadi command repo. Engine `runtime:process` jalanin apa adanya → nol ubah inti.
//
// White-label, multi-OS (Rule #6 no-hardcode: path adapter di-resolve runtime). Consent exec WAJIB
// (clone+install = perintah OS) — owner buka gerbang, bukan AI.
package apps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"flowork-gui/internal/apps/adopt"
	"flowork-gui/internal/apps/cliadapter"
)

// AdoptResult — ringkasan hasil adopt buat owner (GUI / agent).
type AdoptResult struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Runtime    string          `json:"runtime"`
	Detection  adopt.Detection `json:"detection"`
	Installed  bool            `json:"installed"`
	InstallLog string          `json:"install_log,omitempty"`
	Live       bool            `json:"live"`
	Notes      []string        `json:"notes,omitempty"`
}

const adoptInstallTimeout = 10 * time.Minute

var slugBad = regexp.MustCompile(`[^a-z0-9-]+`)

// SlugID — turunin app-id valid (^[a-z0-9][a-z0-9-]{1,40}$) dari nama bebas/URL.
func SlugID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".git")
	if i := strings.LastIndexAny(s, "/\\"); i >= 0 {
		s = s[i+1:] // segmen terakhir URL/path
	}
	s = slugBad.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	if !regexp.MustCompile(`^[a-z0-9]`).MatchString(s) {
		s = "a" + s
	}
	if len(s) > 41 {
		s = s[:41]
	}
	return strings.Trim(s, "-")
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") || strings.HasSuffix(s, ".git")
}

// resolveAdapterBin — indirection biar test bisa nyuntik path adapter palsu (default = adapterBinPath).
var resolveAdapterBin = adapterBinPath

// adapterBinPath — resolve binary fw-app-adapter (core_entry app adopt). Cari di samping
// executable agent, lalu fallback dev. Multi-OS: tambah .exe di Windows. (Rule #6: no-hardcode.)
func adapterBinPath() (string, error) {
	name := "fw-app-adapter"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cands := []string{}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, filepath.Join(d, name), filepath.Join(d, "bin", name))
	}
	if cwd, err := os.Getwd(); err == nil {
		cands = append(cands, filepath.Join(cwd, "bin", name), filepath.Join(cwd, "agent", "bin", name))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", errors.New("binary " + name + " ga ketemu — build dulu: go build -o agent/bin/" + name + " ./cmd/fw-app-adapter")
}

// AdoptRepo — adopt 1 repo jadi app. source = git-URL ATAU folder lokal. approveExec WAJIB true
// (clone+install = exec). skipInstall = lewati install dep (buat dry/test). force = timpa app id sama.
func (m *Manager) AdoptRepo(ctx context.Context, source, id string, approveExec, skipInstall, force bool) (AdoptResult, error) {
	var res AdoptResult
	source = strings.TrimSpace(source)
	if source == "" {
		return res, errors.New("source kosong (kasih git-URL atau path folder)")
	}
	if id == "" {
		id = SlugID(source)
	} else {
		id = SlugID(id)
	}
	if !appIDRe.MatchString(id) {
		return res, errors.New("app id invalid / ga bisa diturunin dari source: " + id)
	}
	if !approveExec {
		return res, errors.New("adopt jalanin perintah OS (clone+install) — butuh approve_exec=1 (consent owner)")
	}
	adapterBin, err := resolveAdapterBin()
	if err != nil {
		return res, err
	}
	if strings.ContainsAny(adapterBin, " ") {
		return res, errors.New("path adapter mengandung spasi (core_entry split by-space): " + adapterBin)
	}

	target := filepath.Join(m.dir, id)
	if st, e := os.Stat(target); e == nil && st.IsDir() && !force {
		return res, errors.New("app '" + id + "' udah ada — pakai force=1 buat timpa")
	}
	repoDir := filepath.Join(target, "repo")

	// staging biar atomic-ish: bikin di tmp, baru pindah. (Sederhana: langsung ke target, rollback kalau gagal.)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return res, fmt.Errorf("mkdir target: %w", err)
	}
	_ = os.RemoveAll(repoDir)

	// ── 1) Ambil kode: clone (git) atau copy (folder lokal) ──────────────────
	if isGitURL(source) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		out, cerr := exec.CommandContext(cctx, "git", "clone", "--depth", "1", source, repoDir).CombinedOutput()
		cancel()
		if cerr != nil {
			_ = os.RemoveAll(target)
			return res, fmt.Errorf("git clone gagal: %v\n%s", cerr, trimTail(string(out), 800))
		}
	} else {
		src, e := filepath.Abs(source)
		if e != nil || !dirExists(src) {
			_ = os.RemoveAll(target)
			return res, errors.New("folder source ga ada: " + source)
		}
		if e := copyTree(src, repoDir); e != nil {
			_ = os.RemoveAll(target)
			return res, fmt.Errorf("copy folder: %w", e)
		}
	}

	// ── 2) Deteksi runtime ───────────────────────────────────────────────────
	det := adopt.Detect(repoDir)
	res.Detection = det
	res.Runtime = string(det.Runtime)
	res.Notes = det.Notes

	// ── 3) Install dep ke folder (kecuali skip) ──────────────────────────────
	if !skipInstall && len(det.InstallCmd) > 0 {
		log, ierr := runInstall(ctx, repoDir, det.InstallCmd)
		res.InstallLog = log
		if ierr != nil {
			// app TETAP dibuat (owner bisa benerin), tapi tandai belum siap.
			res.Notes = append(res.Notes, "INSTALL GAGAL — app dibuat tapi mungkin belum jalan: "+ierr.Error())
		} else {
			res.Installed = true
		}
	}

	// ── 4) Generate adapter.json + manifest.json ─────────────────────────────
	name := id
	if e := writeAdapterJSON(target, det); e != nil {
		_ = os.RemoveAll(target)
		return res, e
	}
	man := Manifest{
		ID: id, Kind: "app", Name: name,
		Description: "Diadopsi dari " + source + " (" + string(det.Runtime) + ") — white-label",
		Version:     "0.1.0", Runtime: "process", CoreEntry: adapterBin,
		Operations: []Op{{
			Name: "run", Tool: true, GUI: false, Mutates: true,
			Description: "Jalanin " + name + " (" + string(det.Runtime) + ") — args: list argumen CLI",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type": "array", "items": map[string]any{"type": "string"},
						"description": "argumen CLI (mis. [\"--help\"] atau [\"<url>\",\"-f\",\"mp4\"])",
					},
				},
			},
		}},
	}
	if e := writeManifestJSON(target, man); e != nil {
		_ = os.RemoveAll(target)
		return res, e
	}

	// ── 5) reloadOne → app LIVE (reuse jalur install yg udah ada) ────────────
	if e := m.reloadOne(id); e != nil {
		_ = os.RemoveAll(target)
		return res, fmt.Errorf("reload app: %w", e)
	}
	res.ID, res.Name, res.Live = id, name, true
	return res, nil
}

// runInstall — jalanin tiap langkah install di repoDir, urut. Stop di langkah pertama yg gagal.
func runInstall(ctx context.Context, repoDir string, steps [][]string) (string, error) {
	var b strings.Builder
	ctx, cancel := context.WithTimeout(ctx, adoptInstallTimeout)
	defer cancel()
	for _, step := range steps {
		if len(step) == 0 {
			continue
		}
		fmt.Fprintf(&b, "$ %s\n", strings.Join(step, " "))
		cmd := exec.CommandContext(ctx, step[0], step[1:]...) // #nosec — step dari detektor (argv, no shell)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		b.Write(out)
		b.WriteByte('\n')
		if err != nil {
			return b.String(), fmt.Errorf("langkah '%s': %w", strings.Join(step, " "), err)
		}
	}
	return b.String(), nil
}

func writeAdapterJSON(target string, det adopt.Detection) error {
	cfg := cliadapter.Config{
		Workdir: "repo",
		Ops: map[string]cliadapter.OpSpec{
			"run": {Cmd: det.RunCmd, ArgStyle: "args_list"},
		},
	}
	return writeJSONFile(filepath.Join(target, cliadapter.ConfigName), cfg)
}

func writeManifestJSON(target string, man Manifest) error {
	return writeJSONFile(filepath.Join(target, "manifest.json"), man)
}

// DetectSource — PREVIEW (dry-run, NO install, NO go-live): deteksi runtime source.
// Folder lokal → deteksi langsung. git-URL → shallow-clone ke temp → deteksi → buang.
// Buat GUI "Deteksi" sebelum owner approve (bagian "setting dikit"). NOL efek samping permanen.
func DetectSource(ctx context.Context, source string) (adopt.Detection, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return adopt.Detection{}, errors.New("source kosong")
	}
	if !isGitURL(source) {
		abs, e := filepath.Abs(source)
		if e != nil || !dirExists(abs) {
			return adopt.Detection{}, errors.New("folder source ga ada: " + source)
		}
		return adopt.Detect(abs), nil
	}
	tmp, e := os.MkdirTemp("", "fw-detect-*")
	if e != nil {
		return adopt.Detection{}, e
	}
	defer os.RemoveAll(tmp)
	repo := filepath.Join(tmp, "repo")
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, cerr := exec.CommandContext(cctx, "git", "clone", "--depth", "1", source, repo).CombinedOutput()
	if cerr != nil {
		return adopt.Detection{}, fmt.Errorf("clone preview gagal: %v\n%s", cerr, trimTail(string(out), 600))
	}
	return adopt.Detect(repo), nil
}
