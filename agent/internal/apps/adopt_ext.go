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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"flowork-gui/internal/apps/adopt"
	"flowork-gui/internal/apps/cliadapter"
	"flowork-gui/internal/apps/httpadapter"
)

// AdoptResult — ringkasan hasil adopt buat owner (GUI / agent).
type AdoptResult struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Runtime    string           `json:"runtime"`
	Detection  adopt.Detection  `json:"detection"`
	Scan       adopt.ScanReport `json:"scan"` // pre-flight: pola berbahaya di kode repo
	Installed  bool             `json:"installed"`
	InstallLog string           `json:"install_log,omitempty"`
	Live       bool             `json:"live"`
	Notes      []string         `json:"notes,omitempty"`
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

// resolveBin — indirection biar test bisa nyuntik path adapter palsu (default = binPath).
var resolveBin = binPath

// binPath — resolve binary adapter ("fw-app-adapter" / "fw-http-adapter") di samping executable
// agent, lalu fallback dev. Multi-OS: +.exe di Windows. (Rule #6: no-hardcode.)
func binPath(name string) (string, error) {
	bin := name
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cands := []string{}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		cands = append(cands, filepath.Join(d, bin), filepath.Join(d, "bin", bin))
	}
	if cwd, err := os.Getwd(); err == nil {
		cands = append(cands, filepath.Join(cwd, "bin", bin), filepath.Join(cwd, "agent", "bin", bin))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", errors.New("binary " + bin + " ga ketemu — build dulu: go build -o agent/bin/" + bin + " ./cmd/" + name)
}

// prepareAdopt — langkah BARENG semua kontrak: validasi + clone/copy + deteksi + install dep ke folder.
// Balik target dir, repoDir, hasil deteksi, dan AdoptResult terisi (Detection/Runtime/Install/Notes).
// Rollback target kalau gagal di tengah.
func (m *Manager) prepareAdopt(ctx context.Context, source, id string, approveExec, skipInstall, force, acceptRisk bool) (string, string, adopt.Detection, AdoptResult, error) {
	var det adopt.Detection
	var res AdoptResult
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", det, res, errors.New("source kosong (kasih git-URL atau path folder)")
	}
	if id == "" {
		id = SlugID(source)
	} else {
		id = SlugID(id)
	}
	if !appIDRe.MatchString(id) {
		return "", "", det, res, errors.New("app id invalid / ga bisa diturunin dari source: " + id)
	}
	if !approveExec {
		return "", "", det, res, errors.New("adopt jalanin perintah OS (clone+install) — butuh approve_exec=1 (consent owner)")
	}
	target := filepath.Join(m.dir, id)
	if st, e := os.Stat(target); e == nil && st.IsDir() && !force {
		return "", "", det, res, errors.New("app '" + id + "' udah ada — pakai force=1 buat timpa")
	}
	repoDir := filepath.Join(target, "repo")
	if e := os.MkdirAll(target, 0o755); e != nil {
		return "", "", det, res, fmt.Errorf("mkdir target: %w", e)
	}
	_ = os.RemoveAll(repoDir)

	// clone (git) atau copy (folder lokal)
	if isGitURL(source) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		out, cerr := exec.CommandContext(cctx, "git", "clone", "--depth", "1", source, repoDir).CombinedOutput()
		cancel()
		if cerr != nil {
			_ = os.RemoveAll(target)
			return "", "", det, res, fmt.Errorf("git clone gagal: %v\n%s", cerr, trimTail(string(out), 800))
		}
	} else {
		src, e := filepath.Abs(source)
		if e != nil || !dirExists(src) {
			_ = os.RemoveAll(target)
			return "", "", det, res, errors.New("folder source ga ada: " + source)
		}
		if e := copyTree(src, repoDir); e != nil {
			_ = os.RemoveAll(target)
			return "", "", det, res, fmt.Errorf("copy folder: %w", e)
		}
	}

	det = adopt.Detect(repoDir)
	res.Detection = det
	res.Runtime = string(det.Runtime)
	res.Notes = det.Notes

	// pre-flight scan (F6): pola berbahaya di kode repo. Critical → BLOCK (rollback) kecuali owner
	// accept sadar (acceptRisk). Install dep BELUM jalan → kode jahat ga ke-eksekusi pas diblok.
	//
	// GERBANG STUDIO per-jenis (ROADMAP_AI_STUDIO F4): repo-adopt diperiksa CARA repo —
	// adopt.ScanRepo, scanner pola-jahat YANG SAMA dipakai verifyAppPack/verifyGenericPack
	// di gerbang plugin. Satu sumber kebenaran pola berbahaya, beda jalur input (folder vs
	// zip). Verdict di-log biar tiap adopt keliatan lewat gerbang Studio.
	res.Scan = adopt.ScanRepo(repoDir)
	gateVerdict := "approved"
	if res.Scan.Warn > 0 {
		gateVerdict = "review"
	}
	if res.Scan.Critical > 0 {
		gateVerdict = "blocked"
	}
	fmt.Fprintf(os.Stderr, "[ai-studio gate] repo-adopt id=%q verdict=%s critical=%d warn=%d scanned=%d accept_risk=%v\n",
		id, gateVerdict, res.Scan.Critical, res.Scan.Warn, res.Scan.Scanned, acceptRisk)
	if res.Scan.Critical > 0 && !acceptRisk {
		_ = os.RemoveAll(target)
		return "", "", det, res, fmt.Errorf("pre-flight scan nemu %d pola BERBAHAYA (critical) di repo — adopt DIBLOKIR. Cek scan.findings; kalau lo yakin aman, ulang dengan accept_risk=1", res.Scan.Critical)
	}
	if res.Scan.Warn > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("scan: %d peringatan (warn) — cek scan.findings", res.Scan.Warn))
	}

	if !skipInstall && len(det.InstallCmd) > 0 {
		log, ierr := runInstall(ctx, repoDir, det.InstallCmd)
		res.InstallLog = log
		if ierr != nil {
			res.Notes = append(res.Notes, "INSTALL GAGAL — app dibuat tapi mungkin belum jalan: "+ierr.Error())
		} else {
			res.Installed = true
		}
	}
	return target, repoDir, det, res, nil
}

// AdoptRepo — adopt repo jadi app kontrak CLI (op "run" → command repo). source = git-URL / folder lokal.
func (m *Manager) AdoptRepo(ctx context.Context, source, id string, approveExec, skipInstall, force, acceptRisk bool) (AdoptResult, error) {
	adapterBin, err := resolveBin("fw-app-adapter")
	if err != nil {
		return AdoptResult{}, err
	}
	if strings.ContainsAny(adapterBin, " ") {
		return AdoptResult{}, errors.New("path adapter mengandung spasi (core_entry split by-space): " + adapterBin)
	}
	target, _, det, res, err := m.prepareAdopt(ctx, source, id, approveExec, skipInstall, force, acceptRisk)
	if err != nil {
		return res, err
	}
	appID := filepath.Base(target)
	if e := writeAdapterJSON(target, det); e != nil {
		_ = os.RemoveAll(target)
		return res, e
	}
	man := Manifest{
		ID: appID, Kind: "app", Name: appID,
		Description: "Diadopsi dari " + source + " (" + string(det.Runtime) + ") — white-label",
		Version:     "0.1.0", Runtime: "process", CoreEntry: adapterBin,
		Operations: []Op{{
			Name: "run", Tool: true, GUI: false, Mutates: true,
			Description: "Jalanin " + appID + " (" + string(det.Runtime) + ") — args: list argumen CLI",
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
	if e := m.reloadOne(appID); e != nil {
		_ = os.RemoveAll(target)
		return res, fmt.Errorf("reload app: %w", e)
	}
	res.ID, res.Name, res.Live = appID, appID, true
	return res, nil
}

// HTTPContract — parameter kontrak HTTP (dari owner via GUI "setting dikit"): cara start server + port + op.
type HTTPContract struct {
	StartCmd        []string                      `json:"start_cmd"`        // argv launch server (mis ["python","main.py"])
	Port            int                           `json:"port"`             // port server listen
	ReadyPath       string                        `json:"ready_path"`       // path cek ready (default "/")
	URLPath         string                        `json:"url_path"`         // path UI manusia (default "/")
	StartTimeoutSec int                           `json:"start_timeout_sec"`
	Ops             map[string]httpadapter.OpSpec `json:"ops"` // op agent → HTTP (boleh kosong: cuma UI)
}

// AdoptHTTPRepo — adopt repo SERVER (web app/API) jadi app kontrak HTTP. Server start saat op pertama;
// manusia buka UI via op "_url", agent panggil op HTTP. Buat MoneyPrinterTurbo dkk (streamlit/fastapi).
func (m *Manager) AdoptHTTPRepo(ctx context.Context, source, id string, hc HTTPContract, approveExec, skipInstall, force, acceptRisk bool) (AdoptResult, error) {
	if len(hc.StartCmd) == 0 || strings.TrimSpace(hc.StartCmd[0]) == "" {
		return AdoptResult{}, errors.New("kontrak http butuh start_cmd (cara jalanin server)")
	}
	if hc.Port <= 0 {
		return AdoptResult{}, errors.New("kontrak http butuh 'port' server valid")
	}
	adapterBin, err := resolveBin("fw-http-adapter")
	if err != nil {
		return AdoptResult{}, err
	}
	if strings.ContainsAny(adapterBin, " ") {
		return AdoptResult{}, errors.New("path adapter mengandung spasi: " + adapterBin)
	}
	target, _, det, res, err := m.prepareAdopt(ctx, source, id, approveExec, skipInstall, force, acceptRisk)
	if err != nil {
		return res, err
	}
	appID := filepath.Base(target)
	if e := writeHTTPAdapterJSON(target, hc); e != nil {
		_ = os.RemoveAll(target)
		return res, e
	}
	ops := []Op{
		{Name: "_alive", GUI: true, Tool: false, Mutates: true, Description: "Nyalakan server " + appID + " (start + tunggu port ready)"},
		{Name: "_url", GUI: true, Tool: false, Description: "URL UI app " + appID + " (buka di tab)"},
	}
	for opName, spec := range hc.Ops {
		ops = append(ops, Op{
			Name: opName, Tool: true, GUI: false, Mutates: true,
			Description: fmt.Sprintf("%s %s (app %s, HTTP)", spec.Method, spec.Path, appID),
		})
	}
	man := Manifest{
		ID: appID, Kind: "app", Name: appID,
		Description: "Diadopsi dari " + source + " (server " + string(det.Runtime) + ", kontrak HTTP) — white-label",
		Version:     "0.1.0", Runtime: "process", CoreEntry: adapterBin, Operations: ops,
	}
	if e := writeManifestJSON(target, man); e != nil {
		_ = os.RemoveAll(target)
		return res, e
	}
	if e := m.reloadOne(appID); e != nil {
		_ = os.RemoveAll(target)
		return res, fmt.Errorf("reload app: %w", e)
	}
	res.ID, res.Name, res.Live = appID, appID, true
	res.Notes = append(res.Notes, "kontrak HTTP — server start saat op pertama; buka UI via op _url")
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

func writeHTTPAdapterJSON(target string, hc HTTPContract) error {
	cfg := httpadapter.Config{
		Workdir: "repo", StartCmd: hc.StartCmd, Port: hc.Port,
		ReadyPath: hc.ReadyPath, URLPath: hc.URLPath, StartTimeoutSec: hc.StartTimeoutSec,
		Ops: hc.Ops,
	}
	return writeJSONFile(filepath.Join(target, httpadapter.ConfigName), cfg)
}

func writeManifestJSON(target string, man Manifest) error {
	return writeJSONFile(filepath.Join(target, "manifest.json"), man)
}

// MCPContract — parameter kontrak MCP: cara start MCP server repo (command WAJIB di allowlist router:
// node/python3/npx/uvx/bun/deno/pnpm/yarn). Entry relatif repo → di-absolut-in (router jalanin TANPA cwd).
type MCPContract struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// AdoptMCPRepo — adopt repo MCP-server: clone+install → DAFTARIN ke MCP-client ROUTER (bukan app sidecar).
// Tool-nya muncul ke semua agent lewat router (tab MCP). Reuse store MCP router yg udah ada.
func (m *Manager) AdoptMCPRepo(ctx context.Context, source, id string, mc MCPContract, approveExec, skipInstall, force, acceptRisk bool) (AdoptResult, error) {
	if strings.TrimSpace(mc.Command) == "" {
		return AdoptResult{}, errors.New("kontrak mcp butuh 'command' (node/python3/npx/uvx/dll — allowlist router)")
	}
	target, repoDir, _, res, err := m.prepareAdopt(ctx, source, id, approveExec, skipInstall, force, acceptRisk)
	if err != nil {
		return res, err
	}
	appID := filepath.Base(target)
	// Router jalanin MCP server TANPA cwd → arg yg berupa file relatif di repo di-absolut-in.
	args := make([]string, len(mc.Args))
	for i, a := range mc.Args {
		if !filepath.IsAbs(a) && fileExists(filepath.Join(repoDir, filepath.FromSlash(a))) {
			args[i] = filepath.Join(repoDir, filepath.FromSlash(a))
		} else {
			args[i] = a
		}
	}
	srv := map[string]any{
		"id": appID, "name": appID, "transport": "stdio",
		"command": mc.Command, "args": args, "env": mc.Env, "enabled": true,
	}
	if e := registerMCP(ctx, srv); e != nil {
		_ = os.RemoveAll(target)
		return res, fmt.Errorf("register MCP ke router: %w", e)
	}
	res.ID, res.Name, res.Live = appID, appID, true
	res.Notes = append(res.Notes, "kontrak MCP — server didaftarin ke MCP-client router; tool-nya muncul ke agent (tab MCP, bukan tab App).")
	return res, nil
}

// registerMCP — indirection biar test bisa nyuntik (default = registerMCPServer, POST ke router).
var registerMCP = registerMCPServer

// registerMCPServer — POST MCP server ke router (loopback, /api/mcp). No-hardcode: URL dari env / default.
func registerMCPServer(ctx context.Context, srv map[string]any) error {
	base := strings.TrimRight(os.Getenv("ROUTER_DEFAULT_URL"), "/")
	if base == "" {
		base = "http://127.0.0.1:2402"
	}
	body, _ := json.Marshal(srv)
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, base+"/api/mcp", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("router HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// DetectSource — PREVIEW (dry-run, NO install, NO go-live): deteksi runtime source.
// Folder lokal → deteksi langsung. git-URL → shallow-clone ke temp → deteksi → buang.
// Buat GUI "Deteksi" sebelum owner approve (bagian "setting dikit"). NOL efek samping permanen.
func DetectSource(ctx context.Context, source string) (adopt.Detection, adopt.ScanReport, adopt.Suggestion, error) {
	preview := func(dir string) (adopt.Detection, adopt.ScanReport, adopt.Suggestion, error) {
		det := adopt.Detect(dir)
		return det, adopt.ScanRepo(dir), adopt.SuggestContract(dir, det), nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return adopt.Detection{}, adopt.ScanReport{}, adopt.Suggestion{}, errors.New("source kosong")
	}
	if !isGitURL(source) {
		abs, e := filepath.Abs(source)
		if e != nil || !dirExists(abs) {
			return adopt.Detection{}, adopt.ScanReport{}, adopt.Suggestion{}, errors.New("folder source ga ada: " + source)
		}
		return preview(abs)
	}
	tmp, e := os.MkdirTemp("", "fw-detect-*")
	if e != nil {
		return adopt.Detection{}, adopt.ScanReport{}, adopt.Suggestion{}, e
	}
	defer os.RemoveAll(tmp)
	repo := filepath.Join(tmp, "repo")
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, cerr := exec.CommandContext(cctx, "git", "clone", "--depth", "1", source, repo).CombinedOutput()
	if cerr != nil {
		return adopt.Detection{}, adopt.ScanReport{}, adopt.Suggestion{}, fmt.Errorf("clone preview gagal: %v\n%s", cerr, trimTail(string(out), 600))
	}
	return preview(repo)
}
