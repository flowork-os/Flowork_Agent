// adopt — DETEKSI runtime repo + rencana isolasi dep (ROADMAP_REPO_TO_APP F2).
// Murni logika (no Manager, no HTTP) → gampang di-test. Sibling apps/adopt_ext.go yang
// nyambungin ke Manager (clone → Detect → tulis manifest+adapter.json → reloadOne).
//
// Prinsip: dep DI FOLDER app (venv / node_modules / target) → hapus folder = bersih total.
// Multi-OS (Rule #6 no-hardcode): path bin runtime di-resolve via GOOS, BUKAN literal.
// White-label: nol identitas corporate.
package adopt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Runtime — jenis repo yang kedeteksi.
type Runtime string

const (
	Python  Runtime = "python"
	Node    Runtime = "node"
	GoLang  Runtime = "go"
	Rust    Runtime = "rust"
	Unknown Runtime = "unknown"
)

// Detection — hasil deteksi 1 repo. InstallCmd dijalanin urut SEBELUM app dipakai (dep ke folder).
// RunCmd = argv default buat op "run" di adapter.json.
type Detection struct {
	Runtime    Runtime    `json:"runtime"`
	Marker     string     `json:"marker"`      // file yg men-trigger deteksi
	Entry      string     `json:"entry"`       // file/target entry kedeteksi
	InstallCmd [][]string `json:"install_cmd"` // langkah install dep ke folder (urut)
	RunCmd     []string   `json:"run_cmd"`     // argv default op "run"
	Notes      []string   `json:"notes"`       // catatan buat owner (bagian "setting dikit")
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// winhost — true kalau host Windows (path bin runtime beda).
func winhost() bool { return runtime.GOOS == "windows" }

// venvBin — path executable di dalam venv, sesuai OS (.venv/bin/x vs .venv\Scripts\x.exe).
func venvBin(name string) string {
	if winhost() {
		return filepath.Join(".venv", "Scripts", name+".exe")
	}
	return filepath.Join(".venv", "bin", name)
}

// DetectorFunc — deteksi 1 runtime custom. Balik (Detection, true) kalau repo cocok.
type DetectorFunc func(repoDir string) (Detection, bool)

// extraDetectors — SWITCH/seam (POLA A): runtime BARU didaftarin lewat sibling _ext.go tanpa
// nyentuh file ini (frozen-friendly). Default kosong = perilaku persis built-in. Dicoba DULUAN
// → bisa override/tambah runtime tanpa unfreeze. (Rule #7: nambah fitur ga buka freeze.)
var extraDetectors []DetectorFunc

// RegisterDetector — daftarin detektor runtime BARU (Python/Node/Go/Rust udah built-in; ini buat
// nambah mis. ruby/php/deno/dotnet lewat sibling). Aman: nil di-skip.
func RegisterDetector(d DetectorFunc) {
	if d != nil {
		extraDetectors = append(extraDetectors, d)
	}
}

// Detect — periksa repoDir, balik Detection. Coba detektor terdaftar (extra) DULU, lalu built-in:
// python → node → go → rust → unknown. Ga pernah error: ga kedeteksi balik Runtime=unknown + notes.
func Detect(repoDir string) Detection {
	for _, d := range extraDetectors {
		if det, ok := d(repoDir); ok {
			return det
		}
	}
	switch {
	case exists(repoDir, "requirements.txt") || exists(repoDir, "pyproject.toml") || exists(repoDir, "setup.py") || hasPyEntry(repoDir):
		return detectPython(repoDir)
	case exists(repoDir, "package.json"):
		return detectNode(repoDir)
	case exists(repoDir, "go.mod"):
		return detectGo(repoDir)
	case exists(repoDir, "Cargo.toml"):
		return detectRust(repoDir)
	default:
		return Detection{
			Runtime: Unknown,
			Notes:   []string{"runtime ga kedeteksi otomatis — owner isi run_cmd manual di adapter.json (setting dikit)"},
		}
	}
}

// ── Python ──────────────────────────────────────────────────────────────────
func hasPyEntry(dir string) bool {
	for _, f := range pyEntryCandidates {
		if exists(dir, f) {
			return true
		}
	}
	return false
}

var pyEntryCandidates = []string{"main.py", "app.py", "cli.py", "run.py", "__main__.py"}

func detectPython(dir string) Detection {
	d := Detection{Runtime: Python}
	install := [][]string{{"python3", "-m", "venv", ".venv"}}
	pip := venvBin("pip")
	switch {
	case exists(dir, "requirements.txt"):
		d.Marker = "requirements.txt"
		install = append(install, []string{pip, "install", "-r", "requirements.txt"})
	case exists(dir, "pyproject.toml"):
		d.Marker = "pyproject.toml"
		install = append(install, []string{pip, "install", "."})
	case exists(dir, "setup.py"):
		d.Marker = "setup.py"
		install = append(install, []string{pip, "install", "."})
	default:
		d.Marker = "*.py"
		d.Notes = append(d.Notes, "ga ada requirements.txt — venv dibikin kosong (kalau butuh dep, owner tambah)")
	}
	d.InstallCmd = install
	for _, c := range pyEntryCandidates {
		if exists(dir, c) {
			d.Entry = c
			break
		}
	}
	if d.Entry == "" {
		d.Notes = append(d.Notes, "entry .py ga ketebak — owner set run_cmd manual")
		d.RunCmd = []string{venvBin("python")}
	} else {
		d.RunCmd = []string{venvBin("python"), d.Entry}
	}
	return d
}

// ── Node ────────────────────────────────────────────────────────────────────
func detectNode(dir string) Detection {
	d := Detection{Runtime: Node, Marker: "package.json", InstallCmd: [][]string{{"npm", "install"}}}
	pkg := readPackageJSON(dir)
	switch {
	case pkg.binPath() != "":
		d.Entry = pkg.binPath()
		d.RunCmd = []string{"node", pkg.binPath()}
	case pkg.Scripts["start"] != "":
		d.Entry = "npm start"
		d.RunCmd = []string{"npm", "start"}
	case pkg.Main != "":
		d.Entry = pkg.Main
		d.RunCmd = []string{"node", pkg.Main}
	default:
		d.Entry = "index.js"
		d.RunCmd = []string{"node", "index.js"}
		d.Notes = append(d.Notes, "entry node ga jelas dari package.json — default index.js, owner cek")
	}
	return d
}

type packageJSON struct {
	Main    string            `json:"main"`
	Bin     json.RawMessage   `json:"bin"`
	Scripts map[string]string `json:"scripts"`
}

// binPath — ambil 1 path dari field "bin" (bisa string atau object {name:path}).
func (p packageJSON) binPath() string {
	if len(p.Bin) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(p.Bin, &s) == nil && s != "" {
		return s
	}
	var m map[string]string
	if json.Unmarshal(p.Bin, &m) == nil {
		for _, v := range m {
			if v != "" {
				return v
			}
		}
	}
	return ""
}

func readPackageJSON(dir string) packageJSON {
	var p packageJSON
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err == nil {
		_ = json.Unmarshal(raw, &p)
	}
	return p
}

// ── Go ──────────────────────────────────────────────────────────────────────
func detectGo(dir string) Detection {
	bin := "app"
	if winhost() {
		bin = "app.exe"
	}
	return Detection{
		Runtime:    GoLang,
		Marker:     "go.mod",
		Entry:      bin,
		InstallCmd: [][]string{{"go", "build", "-o", bin, "."}},
		RunCmd:     []string{"./" + bin}, // ber-separator → adapter resolve ke workdir (bukan PATH)
	}
}

// ── Rust ────────────────────────────────────────────────────────────────────
func detectRust(dir string) Detection {
	name := cargoPackageName(dir)
	if name == "" {
		name = "app"
	}
	binName := name
	if winhost() {
		binName += ".exe"
	}
	return Detection{
		Runtime:    Rust,
		Marker:     "Cargo.toml",
		Entry:      name,
		InstallCmd: [][]string{{"cargo", "build", "--release"}},
		RunCmd:     []string{"./target/release/" + binName}, // ber-separator → resolve ke workdir
	}
}

// cargoPackageName — parsing minimal [package] name = "x" dari Cargo.toml (tanpa lib TOML).
func cargoPackageName(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if err != nil {
		return ""
	}
	inPkg := false
	for _, ln := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "[") {
			inPkg = t == "[package]"
			continue
		}
		if inPkg && strings.HasPrefix(t, "name") {
			if i := strings.Index(t, "="); i >= 0 {
				return strings.Trim(strings.TrimSpace(t[i+1:]), `"'`)
			}
		}
	}
	return ""
}
