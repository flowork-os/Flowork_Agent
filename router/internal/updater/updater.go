// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — audit pass surface review.

// Auto-updater (GitHub releases → binary swap).

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

// Repo is "owner/name" of the GitHub repo whose releases we track.
var Repo = "flowork-os/Flowork-OS"

// CurrentVersion is the running build; the dispatcher sets it from main.
var CurrentVersion = "0.0.0"

// Release describes one GitHub release.
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

// Asset is a release binary.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

// LatestRelease fetches the newest non-draft non-prerelease release.
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

// AssetForPlatform picks the asset whose name matches the current OS/arch.
// Asset names should embed `<os>-<arch>` (e.g. flow-router-linux-amd64). When
// none matches, returns nil.
func AssetForPlatform(rel *Release) *Asset {
	target := strings.ToLower(runtime.GOOS + "-" + runtime.GOARCH)
	for i := range rel.Assets {
		if strings.Contains(strings.ToLower(rel.Assets[i].Name), target) {
			return &rel.Assets[i]
		}
	}
	return nil
}

// IsNewer reports remote > current (very simple semver compare: split on dot,
// numeric prefix lex). "v" prefix tolerated.
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

// DownloadAsset downloads asset to <exec>.new. Returns (path, sha256-hex, err).
// The .new file lives next to the running executable so the rename is on the
// same filesystem (atomic).
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

// Swap atomically renames <new> over the running executable. On Windows we
// CAN'T overwrite the file while running; in that case Swap returns
// ErrSwapDeferred and the caller should leave the .new in place — the next
// launch will prefer it (see PreferNewOnLaunch helper).
func Swap(newPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Try the move; if it fails because of file-lock, leave for next launch.
		if err := os.Rename(newPath, exe); err != nil {
			return ErrSwapDeferred
		}
		return nil
	}
	return os.Rename(newPath, exe)
}

// ErrSwapDeferred is returned by Swap when overwrite cannot happen now (Win).
var ErrSwapDeferred = fmt.Errorf("swap deferred to next launch")

// PreferNewOnLaunch detects <exec>.new from a previous deferred swap and
// completes the rename, then returns true. Callers run this in main() before
// the rest of bootstrap.
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

// RestartSelf re-execs the running binary with the same args + env. The
// caller should call this AFTER Swap returned nil. On Windows it spawns the
// new process and exits the old one; on Unix it `syscall.Exec`s.
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return restartImpl(exe)
}

// scratchPath returns a sibling path for tests that don't want to touch /tmp.
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
