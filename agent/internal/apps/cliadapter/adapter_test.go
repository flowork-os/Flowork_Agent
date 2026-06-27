package cliadapter

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCfg — bikin folder app sementara + adapter.json.
func writeCfg(t *testing.T, cfg Config) string {
	t.Helper()
	dir := t.TempDir()
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, ConfigName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runOps — drive Run dgn beberapa request, balik daftar response (1 baris/req).
func runOps(t *testing.T, dir string, reqs ...string) []map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(reqs, "\n") + "\n")
	var out bytes.Buffer
	if err := Run(in, &out, dir); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var resps []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("resp bukan JSON: %q (%v)", line, err)
		}
		resps = append(resps, m)
	}
	return resps
}

// resultField — ambil result.<key> dari response.
func resultField(t *testing.T, resp map[string]any, key string) any {
	t.Helper()
	if e, ok := resp["error"]; ok {
		t.Fatalf("response error: %v", e)
	}
	res, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result bukan object: %v", resp["result"])
	}
	return res[key]
}

func TestSimpleOp(t *testing.T) {
	dir := writeCfg(t, Config{
		Ops: map[string]OpSpec{
			"ping": {Cmd: []string{"echo", "hello"}},
		},
	})
	resps := runOps(t, dir, `{"op":"ping","args":{}}`)
	if len(resps) != 1 {
		t.Fatalf("mau 1 response, dapet %d", len(resps))
	}
	if got := resultField(t, resps[0], "stdout"); !strings.Contains(got.(string), "hello") {
		t.Fatalf("stdout = %q, mau berisi 'hello'", got)
	}
	if got := resultField(t, resps[0], "exit"); got.(float64) != 0 {
		t.Fatalf("exit = %v, mau 0", got)
	}
	if sv, _ := resps[0]["state_version"].(float64); sv != 1 {
		t.Fatalf("state_version = %v, mau 1", resps[0]["state_version"])
	}
}

func TestPlaceholderSubstitution(t *testing.T) {
	dir := writeCfg(t, Config{
		Ops: map[string]OpSpec{
			"greet": {Cmd: []string{"echo", "halo-{name}"}},
		},
	})
	resps := runOps(t, dir, `{"op":"greet","args":{"name":"aola"}}`)
	got := resultField(t, resps[0], "stdout").(string)
	if !strings.Contains(got, "halo-aola") {
		t.Fatalf("stdout = %q, mau berisi 'halo-aola'", got)
	}
}

func TestFlagsStyle(t *testing.T) {
	// arg_style=flags → key non-placeholder jadi --key value. echo balikin argumen.
	dir := writeCfg(t, Config{
		Ops: map[string]OpSpec{
			"run": {Cmd: []string{"echo"}, ArgStyle: "flags"},
		},
	})
	resps := runOps(t, dir, `{"op":"run","args":{"verbose":"1"}}`)
	got := resultField(t, resps[0], "stdout").(string)
	if !strings.Contains(got, "--verbose") || !strings.Contains(got, "1") {
		t.Fatalf("stdout = %q, mau berisi '--verbose 1'", got)
	}
}

func TestArgsListStyle(t *testing.T) {
	// arg_style=args_list → elemen args["args"] di-append apa adanya (bungkus CLI).
	dir := writeCfg(t, Config{
		Ops: map[string]OpSpec{
			"run": {Cmd: []string{"echo", "yt-dlp"}, ArgStyle: "args_list"},
		},
	})
	resps := runOps(t, dir, `{"op":"run","args":{"args":["https://x.test/v","-f","mp4"]}}`)
	got := resultField(t, resps[0], "stdout").(string)
	if !strings.Contains(got, "https://x.test/v") || !strings.Contains(got, "-f") || !strings.Contains(got, "mp4") {
		t.Fatalf("stdout = %q, mau berisi argumen list", got)
	}
}

func TestUnknownOp(t *testing.T) {
	dir := writeCfg(t, Config{Ops: map[string]OpSpec{"x": {Cmd: []string{"echo"}}}})
	resps := runOps(t, dir, `{"op":"nope","args":{}}`)
	if _, ok := resps[0]["error"]; !ok {
		t.Fatalf("mau error utk op tak dikenal, dapet %v", resps[0])
	}
}

func TestNonzeroExitCaptured(t *testing.T) {
	// `false` exit 1 → BUKAN error adapter, tapi result.exit=1 (exit-code app ke-tangkap).
	dir := writeCfg(t, Config{Ops: map[string]OpSpec{"fail": {Cmd: []string{"false"}}}})
	resps := runOps(t, dir, `{"op":"fail","args":{}}`)
	if _, ok := resps[0]["error"]; ok {
		t.Fatalf("exit-code app ga boleh jadi error adapter: %v", resps[0])
	}
	if got := resultField(t, resps[0], "exit"); got.(float64) != 1 {
		t.Fatalf("exit = %v, mau 1", got)
	}
}

func TestBadConfig(t *testing.T) {
	dir := t.TempDir() // no adapter.json
	var out bytes.Buffer
	if err := Run(strings.NewReader(`{"op":"x"}`+"\n"), &out, dir); err == nil {
		t.Fatal("mau error config (adapter.json absen), dapet nil")
	}
}
