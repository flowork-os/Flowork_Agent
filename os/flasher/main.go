// Flowork USB Maker — the PUBLIC, user-facing flasher. Fetches the latest Flowork OS release
// from the public channel, downloads the (compressed) image, and writes it to a USB stick.
// No build, no source — users just download + flash. Stdlib only. Flashing needs root → sudo.
package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

//go:embed web
var webFS embed.FS

var (
	addr      = flag.String("addr", "127.0.0.1:8800", "listen address")
	repo      = flag.String("repo", "flowork-os/Flowork-OS", "public release repo (owner/name)")
	ghAPI     = flag.String("api", "https://api.github.com", "GitHub API base (override for testing/mirror)")
	noBrowser = flag.Bool("no-browser", false, "do not auto-open a browser")
)

func main() {
	flag.Parse()
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/latest", apiLatest)
	mux.HandleFunc("/api/devices", apiDevices)
	mux.HandleFunc("/api/flash", apiFlash)

	fmt.Printf("Flowork USB Maker → http://%s   (channel: %s)\n", *addr, *repo)
	if os.Geteuid() != 0 {
		fmt.Println("note: flashing needs root — re-run with sudo if Flash fails.")
	}
	if !*noBrowser {
		go openBrowser("http://" + *addr)
	}
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ── latest release ───────────────────────────────────────────────────────
var urlRe = regexp.MustCompile(`"browser_download_url"\s*:\s*"([^"]+)"`)
var tagRe = regexp.MustCompile(`"tag_name"\s*:\s*"([^"]+)"`)

func apiLatest(w http.ResponseWriter, r *http.Request) {
	body, err := httpGet(*ghAPI + "/repos/" + *repo + "/releases/latest")
	if err != nil {
		writeJSON(w, map[string]any{"error": "cannot reach the release channel: " + err.Error()})
		return
	}
	tag := ""
	if m := tagRe.FindStringSubmatch(body); m != nil {
		tag = strings.TrimPrefix(m[1], "v")
	}
	var image, manifest string
	for _, m := range urlRe.FindAllStringSubmatch(body, -1) {
		u := m[1]
		switch {
		case strings.HasSuffix(u, ".usb.img.zst"):
			image = u
		case strings.HasSuffix(u, "manifest.json"):
			manifest = u
		}
	}
	// sha256 of the image, if the manifest carries it.
	sha := ""
	if manifest != "" {
		if mb, err := httpGet(manifest); err == nil {
			var mf struct {
				OsImage *struct {
					Asset, Sha256 string
				} `json:"os_image"`
			}
			if json.Unmarshal([]byte(mb), &mf) == nil && mf.OsImage != nil {
				sha = mf.OsImage.Sha256
			}
		}
	}
	writeJSON(w, map[string]any{
		"version": tag, "image": image, "sha256": sha,
		"found": image != "",
	})
}

// ── devices (removable/USB only — system disk never offered) ─────────────
func apiDevices(w http.ResponseWriter, r *http.Request) {
	raw, err := exec.Command("lsblk", "-J", "-b", "-o", "NAME,SIZE,TYPE,TRAN,RM,MODEL,PATH").Output()
	if err != nil {
		writeJSON(w, map[string]any{"devices": []any{}, "error": err.Error()})
		return
	}
	var parsed struct {
		Blockdevices []struct {
			Name, Type, Tran, Model, Path string
			Size                          int64
			Rm                            bool
		} `json:"blockdevices"`
	}
	_ = json.Unmarshal(raw, &parsed)
	devs := []map[string]any{}
	for _, b := range parsed.Blockdevices {
		if b.Type != "disk" || !(b.Rm || b.Tran == "usb") {
			continue
		}
		p := b.Path
		if p == "" {
			p = "/dev/" + b.Name
		}
		devs = append(devs, map[string]any{"path": p, "size": b.Size, "model": strings.TrimSpace(b.Model), "tran": b.Tran})
	}
	writeJSON(w, map[string]any{"devices": devs})
}

// ── flash: download .img.zst → verify sha256 → zstd -dc | dd ──────────────
func apiFlash(w http.ResponseWriter, r *http.Request) {
	var req struct{ Image, Device, Sha256 string }
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Image == "" || req.Device == "" {
		http.Error(w, "need image + device", 400)
		return
	}
	if !isFlashSafe(req.Device) {
		http.Error(w, "refusing: "+req.Device+" is not a removable/USB disk", 400)
		return
	}
	// Pipe: curl image | (tee-verify) ... we keep it robust + simple: download, verify, write.
	// zstd streams the decompress straight to dd so we never need 6 GB of free disk.
	script := fmt.Sprintf(`set -o pipefail
TMP="$(mktemp /tmp/flowork-img.XXXXXX.zst)"
trap 'rm -f "$TMP"' EXIT
echo "Downloading image…"
curl -fSL --progress-bar %q -o "$TMP" 2>&1
`, req.Image)
	if req.Sha256 != "" {
		script += fmt.Sprintf(`echo "Verifying checksum…"
echo "%s  $TMP" | sha256sum -c - || { echo "CHECKSUM MISMATCH — refusing to flash"; exit 1; }
`, req.Sha256)
	}
	dd := "dd of=" + req.Device + " bs=4M status=progress conv=fsync"
	if os.Geteuid() != 0 {
		dd = "sudo " + dd
	}
	script += fmt.Sprintf(`echo "Writing to %s…"
zstd -dc "$TMP" | %s
echo "Flushing…"; sync
`, req.Device, dd)
	streamCmd(w, exec.Command("bash", "-c", script))
}

func isFlashSafe(dev string) bool {
	out, err := exec.Command("lsblk", "-dno", "RM,TRAN,TYPE", dev).Output()
	if err != nil {
		return false
	}
	f := strings.Fields(string(out))
	rm, tran, typ := "", "", ""
	switch len(f) {
	case 3:
		rm, tran, typ = f[0], f[1], f[2]
	case 2:
		rm, typ = f[0], f[1]
	}
	return typ == "disk" && (rm == "1" || tran == "usb")
}

// ── helpers ──────────────────────────────────────────────────────────────
func httpGet(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func streamCmd(w http.ResponseWriter, cmd *exec.Cmd) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", 500)
		return
	}
	pr, pw, _ := os.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(w, "data: ERROR: %s\n\n", err)
		fl.Flush()
		return
	}
	pw.Close()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", sc.Text())
		fl.Flush()
	}
	rc := 0
	if err := cmd.Wait(); err != nil {
		rc = 1
	}
	fmt.Fprintf(w, "data: __DONE__ %d\n\n", rc)
	fl.Flush()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func openBrowser(url string) {
	for _, c := range [][]string{{"xdg-open", url}, {"open", url}} {
		if _, err := exec.LookPath(c[0]); err == nil {
			_ = exec.Command(c[0], c[1:]...).Start()
			return
		}
	}
}
