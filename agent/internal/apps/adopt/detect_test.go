package adopt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkrepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDetectPythonRequirements(t *testing.T) {
	dir := mkrepo(t, map[string]string{
		"requirements.txt": "requests\n",
		"main.py":          "print('hi')\n",
	})
	d := Detect(dir)
	if d.Runtime != Python {
		t.Fatalf("runtime = %s, mau python", d.Runtime)
	}
	if d.Entry != "main.py" {
		t.Fatalf("entry = %s, mau main.py", d.Entry)
	}
	if len(d.InstallCmd) < 2 {
		t.Fatalf("install cmd mau ada venv + pip install, dapet %v", d.InstallCmd)
	}
	// run_cmd harus nunjuk python di venv (path bin OS-aware).
	if !strings.Contains(d.RunCmd[0], ".venv") || len(d.RunCmd) != 2 {
		t.Fatalf("run_cmd = %v, mau [<venv-python> main.py]", d.RunCmd)
	}
}

func TestDetectNodeBin(t *testing.T) {
	dir := mkrepo(t, map[string]string{
		"package.json": `{"name":"x","bin":{"x":"cli.js"},"main":"index.js"}`,
	})
	d := Detect(dir)
	if d.Runtime != Node {
		t.Fatalf("runtime = %s, mau node", d.Runtime)
	}
	if d.Entry != "cli.js" {
		t.Fatalf("entry = %s, mau cli.js (dari bin)", d.Entry)
	}
	if d.RunCmd[0] != "node" || d.RunCmd[1] != "cli.js" {
		t.Fatalf("run_cmd = %v, mau [node cli.js]", d.RunCmd)
	}
}

func TestDetectNodeStartScript(t *testing.T) {
	dir := mkrepo(t, map[string]string{
		"package.json": `{"name":"x","scripts":{"start":"node server.js"}}`,
	})
	d := Detect(dir)
	if d.Runtime != Node || d.RunCmd[0] != "npm" || d.RunCmd[1] != "start" {
		t.Fatalf("mau npm start, dapet %v", d.RunCmd)
	}
}

func TestDetectGo(t *testing.T) {
	dir := mkrepo(t, map[string]string{"go.mod": "module x\n\ngo 1.25\n"})
	d := Detect(dir)
	if d.Runtime != GoLang {
		t.Fatalf("runtime = %s, mau go", d.Runtime)
	}
	if len(d.InstallCmd) != 1 || d.InstallCmd[0][0] != "go" {
		t.Fatalf("install mau go build, dapet %v", d.InstallCmd)
	}
}

func TestDetectRustName(t *testing.T) {
	dir := mkrepo(t, map[string]string{
		"Cargo.toml": "[package]\nname = \"ripgrep\"\nversion = \"1.0\"\n",
	})
	d := Detect(dir)
	if d.Runtime != Rust {
		t.Fatalf("runtime = %s, mau rust", d.Runtime)
	}
	if !strings.Contains(d.RunCmd[0], "ripgrep") {
		t.Fatalf("run_cmd = %v, mau nunjuk target/release/ripgrep", d.RunCmd)
	}
}

func TestDetectUnknown(t *testing.T) {
	dir := mkrepo(t, map[string]string{"README.md": "halo"})
	d := Detect(dir)
	if d.Runtime != Unknown {
		t.Fatalf("runtime = %s, mau unknown", d.Runtime)
	}
	if len(d.Notes) == 0 {
		t.Fatal("unknown mau ada notes buat owner")
	}
}

// switch: detektor terdaftar (registry) dicoba duluan; non-match fallback ke built-in.
func TestRegisterDetector(t *testing.T) {
	old := extraDetectors
	t.Cleanup(func() { extraDetectors = old })
	RegisterDetector(func(dir string) (Detection, bool) {
		if exists(dir, "deno.json") {
			return Detection{Runtime: "deno", Marker: "deno.json", RunCmd: []string{"deno", "run", "main.ts"}}, true
		}
		return Detection{}, false
	})
	if got := Detect(mkrepo(t, map[string]string{"deno.json": "{}"})); got.Runtime != "deno" {
		t.Fatalf("runtime = %s, mau deno (dari registry)", got.Runtime)
	}
	// non-match → fallback built-in go.
	if got := Detect(mkrepo(t, map[string]string{"go.mod": "module x\n"})); got.Runtime != GoLang {
		t.Fatalf("non-match registry mau fallback go, dapet %s", got.Runtime)
	}
}

// prioritas: requirements.txt (python) menang walau ada file lain non-marker.
func TestDetectPriority(t *testing.T) {
	dir := mkrepo(t, map[string]string{
		"pyproject.toml": "[project]\nname='x'\n",
		"app.py":         "x=1\n",
	})
	d := Detect(dir)
	if d.Runtime != Python || d.Marker != "pyproject.toml" {
		t.Fatalf("mau python/pyproject, dapet %s/%s", d.Runtime, d.Marker)
	}
}
