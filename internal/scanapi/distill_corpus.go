// distill_corpus.go — distilasi dari CORPUS (exploitdb dst) → check. Nyisir wing
// via router /api/brain/wing → tiap drawer (entri exploit) → LLM bikin template
// DETEKSI → DEDUP (vs nuclei publik by-CVE + vs yg udah didistilasi) → validate →
// ingest. "Bener-bener kuat": cuma generate yg BARU (moat), resumable (offset),
// quality-filter, tetep lewat gerbang yg sama. Detection-only (anti senjata).

package scanapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/routerclient"
)

// --- dedup vs nuclei publik (jangan regenerate 13rb yg udah ada; moat = yg BARU) ---
var (
	pubIDMu    sync.Mutex
	pubIDSet   map[string]bool
	pubIDReady bool
)

// publicTemplateIDs — set basename (lowercase, tanpa .yaml) SEMUA template nuclei
// publik → dedup O(1). Cached (walk sekali).
func publicTemplateIDs() map[string]bool {
	pubIDMu.Lock()
	defer pubIDMu.Unlock()
	if pubIDReady {
		return pubIDSet
	}
	pubIDSet = map[string]bool{}
	if dir := nucleiTemplatesDir(); dir != "" {
		_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
				pubIDSet[strings.ToLower(strings.TrimSuffix(d.Name(), ".yaml"))] = true
			}
			return nil
		})
	}
	pubIDReady = true
	return pubIDSet
}

var (
	reEDB = regexp.MustCompile(`EDB-(\d+)`)
	reCVE = regexp.MustCompile(`(?i)CVE-(\d{4}-\d{3,7})`)
)

func parseEDBCVE(content string) (edb, cve string) {
	if m := reEDB.FindStringSubmatch(content); m != nil {
		edb = m[1]
	}
	if m := reCVE.FindStringSubmatch(content); m != nil {
		cve = "cve-" + strings.ToLower(m[1])
	}
	return
}

type wingDrawer struct {
	DrawerID string `json:"drawer_id"`
	Room     string `json:"room"`
	Content  string `json:"content"`
}

// fetchWingDrawers — GET router /api/brain/wing (read-only enumerate corpus).
// roomLike (mis. "%webapps%") → fetch cuma kategori http-detectable (anti boros).
func fetchWingDrawers(ctx context.Context, wing, roomLike string, limit, offset int) ([]wingDrawer, error) {
	u := routerclient.DefaultRouterURL + "/api/brain/wing?wing=" + url.QueryEscape(wing) +
		"&limit=" + strconv.Itoa(limit) + "&offset=" + strconv.Itoa(offset)
	if roomLike != "" {
		u += "&room_like=" + url.QueryEscape(roomLike)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var out struct {
		Drawers []wingDrawer `json:"drawers"`
	}
	if e := json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out.Drawers, nil
}

const distillCorpusUser = `Raw Exploit-DB entry below. Produce ONE Nuclei v3 template that DETECTS whether a target is affected (version banner, vulnerable path, or response fingerprint) — DETECTION ONLY, do NOT weaponize. Prefer an http GET/HEAD probe with a precise matcher (status + word/regex, matchers-condition: and). If not remotely HTTP-detectable, emit the most reasonable passive http fingerprint. author: flowork.

--- ENTRY ---
`

// distillDrawer — 1 drawer corpus → template tervalidasi (LLM forced-tool + safety).
func distillDrawer(ctx context.Context, model, content string) (yaml string, err error) {
	args, e := distillLLM(ctx, model, distillSystemPrompt, distillCorpusUser+content)
	if e != nil {
		return "", e
	}
	var out struct {
		YAML string `json:"yaml"`
	}
	if e := json.Unmarshal(args, &out); e != nil {
		return "", fmt.Errorf("decode emit: %w", e)
	}
	if strings.TrimSpace(out.YAML) == "" {
		return "", fmt.Errorf("output kosong")
	}
	low := strings.ToLower(out.YAML)
	for _, bad := range []string{"code:", "method: delete", "method: put", "javascript:", "- engine:"} {
		if strings.Contains(low, bad) {
			return "", fmt.Errorf("ditolak safety: %q", bad)
		}
	}
	return out.YAML, nil
}

// ScannerDistillCorpusHandler — POST {wing?, limit?, offset?, model?}: nyisir corpus
// → generate check BARU (dedup) → validate → ingest. Resumable via next_offset.
func ScannerDistillCorpusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
			return
		}
		var body struct {
			Wing     string `json:"wing"`
			RoomLike string `json:"room_like"`
			Limit    int    `json:"limit"`
			Offset   int    `json:"offset"`
			Model    string `json:"model"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body)
		wing := strings.TrimSpace(body.Wing)
		if wing == "" {
			wing = "exploitdb"
		}
		if body.Limit <= 0 || body.Limit > 60 {
			body.Limit = 30
		}
		model := strings.TrimSpace(body.Model)
		if model == "" {
			model = distillModelDefault
		}
		dir := privateChecksDir()
		if dir == "" {
			tfWriteJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "dir template nuclei ga ketemu"})
			return
		}
		_ = os.MkdirAll(dir, 0o755)
		pub := publicTemplateIDs()

		ctxF, cancelF := context.WithTimeout(r.Context(), 30*time.Second)
		drawers, ferr := fetchWingDrawers(ctxF, wing, strings.TrimSpace(body.RoomLike), body.Limit, body.Offset)
		cancelF()
		if ferr != nil {
			tfWriteJSON(w, http.StatusBadGateway, map[string]any{"error": "fetch wing: " + ferr.Error()})
			return
		}

		var added, dupPub, dupLocal, skipType, genFail, invalid int
		for _, dr := range drawers {
			room := strings.ToLower(dr.Room)
			if strings.Contains(room, "local") || strings.Contains(room, "dos") || strings.Contains(room, "shellcode") {
				skipType++ // ga ke-deteksi http
				continue
			}
			edb, cve := parseEDBCVE(dr.Content)
			if cve != "" && pub[cve] {
				dupPub++ // udah di-cover nuclei publik → skip (anti redundan)
				continue
			}
			var name string
			switch {
			case edb != "":
				name = "flowork-edb-" + edb
			case cve != "":
				name = "flowork-" + cve
			default:
				name = "flowork-" + sanitizeCheckID(dr.DrawerID)
			}
			if !validCheckName(name) {
				continue
			}
			if _, e := os.Stat(filepath.Join(dir, name+".yaml")); e == nil {
				dupLocal++ // udah didistilasi → resumable tanpa dobel
				continue
			}
			ctx, cancel := context.WithTimeout(r.Context(), 130*time.Second)
			yaml, gerr := distillDrawer(ctx, model, dr.Content)
			if gerr != nil {
				cancel()
				genFail++
				continue
			}
			if verr := ingestValidatedCheck(dir, name, yaml); verr != nil {
				cancel()
				invalid++
				continue
			}
			cancel()
			added++
		}
		if added > 0 {
			resetNucleiPackCache()
		}
		tfWriteJSON(w, 0, map[string]any{
			"ok": true, "wing": wing, "scanned": len(drawers), "added": added,
			"dup_public": dupPub, "dup_local": dupLocal, "skip_type": skipType,
			"gen_fail": genFail, "invalid": invalid,
			"next_offset": body.Offset + len(drawers),
		})
	}
}
