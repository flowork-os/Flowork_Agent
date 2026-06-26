// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var Repo = "flowork-os/Flowork-OS"

var CurrentVersion = "0.0.0"

type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

func LatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("github %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var r Release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func AssetForPlatform(rel *Release) *Asset {
	target := strings.ToLower(runtime.GOOS + "-" + runtime.GOARCH)
	for i := range rel.Assets {
		if strings.Contains(strings.ToLower(rel.Assets[i].Name), target) {
			return &rel.Assets[i]
		}
	}
	return nil
}

func IsNewer(remote, current string) bool {
	r := strings.TrimPrefix(strings.TrimPrefix(remote, "v"), "V")
	c := strings.TrimPrefix(strings.TrimPrefix(current, "v"), "V")
	rp := strings.Split(r, ".")
	cp := strings.Split(c, ".")
	for i := 0; i < max(len(rp), len(cp)); i++ {
		var rv, cv int
		if i < len(rp) {
			rv = numPrefix(rp[i])
		}
		if i < len(cp) {
			cv = numPrefix(cp[i])
		}
		if rv != cv {
			return rv > cv
		}
	}
	return false
}

func numPrefix(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func DownloadAsset(ctx context.Context, asset *Asset) (string, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	dest := exe + ".new"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("download %d", resp.StatusCode)
	}
	tmpFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", "", err
	}
	defer tmpFile.Close()
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body); err != nil {
		_ = os.Remove(dest)
		return "", "", err
	}
	return dest, hex.EncodeToString(hasher.Sum(nil)), nil
}

func Swap(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {

		if err := os.Rename(newPath, exe); err != nil {
			return ErrSwapDeferred
		}
		return nil
	}
	return os.Rename(newPath, exe)
}

var ErrSwapDeferred = fmt.Errorf("swap deferred to next launch")

func PreferNewOnLaunch() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	candidate := exe + ".new"
	if _, err := os.Stat(candidate); err != nil {
		return false
	}
	if err := os.Rename(candidate, exe); err != nil {
		return false
	}
	return true
}

func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return restartImpl(exe)
}

func scratchPath(dir, name string) string {
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, name)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
