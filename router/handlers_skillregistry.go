// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval. Owner: Aola Sahidin (Mr.Dev).
// Locked 2026-06-17 · P2 fase-2b registry endpoints, owner-approved, E2E tested live.
//
// handlers_skillregistry.go — P2 A2 fase-2b: endpoint transport registry komunitas.
//
//   GET  /api/skills/registry/status            — repo + apakah token publish ada.
//   GET  /api/skills/registry/browse            — katalog skill di registry (public, no token).
//   POST /api/skills/registry/pull?name=         — download → VERIFY (sig+content) → import. (loopback)
//   POST /api/skills/registry/publish?skill=     — karma-gate → sign → push ke registry. (loopback+token)
//
// 3 gerbang tetap di-enforce: pull verify provenance+content SEBELUM import (registry untrusted);
// publish lewat CanPublish (kebukti bagus lokal) + sign (provenance). Engine: internal/skillregistry.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/mesh"
	"github.com/flowork-os/flowork_Router/internal/skillpack"
	"github.com/flowork-os/flowork_Router/internal/skillregistry"
	"github.com/flowork-os/flowork_Router/internal/store"
)

var registrySkillNameRe = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeSkillName(s string) string {
	return strings.Trim(registrySkillNameRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-"), "-")
}

// readOneSkill baca SKILL.md by-name dari dir (flat <name>.md atau <name>/SKILL.md).
func readOneSkill(dir, name string) (string, bool) {
	if dir == "" || name == "" {
		return "", false
	}
	for _, p := range []string{filepath.Join(dir, name+".md"), filepath.Join(dir, name, "SKILL.md")} {
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			return string(b), true
		}
	}
	return "", false
}

func frontmatterDesc(content string) string {
	for _, ln := range strings.Split(content, "\n") {
		if s := strings.TrimSpace(ln); strings.HasPrefix(strings.ToLower(s), "description:") {
			return strings.TrimSpace(s[len("description:"):])
		}
	}
	return ""
}

// skillRegistryStatusHandler — GET /api/skills/registry/status
func skillRegistryStatusHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"repo": skillregistry.Repo(), "can_publish": skillregistry.HasToken(),
	})
}

// skillRegistryBrowseHandler — GET /api/skills/registry/browse
func skillRegistryBrowseHandler(w http.ResponseWriter, r *http.Request) {
	idx, err := skillregistry.FetchIndex(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repo": skillregistry.Repo(), "registry_version": idx.RegistryVersion,
		"updated_at": idx.UpdatedAt, "skills": idx.Skills, "count": len(idx.Skills),
	})
}

// skillRegistryPullHandler — POST /api/skills/registry/pull?name=
func skillRegistryPullHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if !loopbackOnly(w, r) {
		return
	}
	name := sanitizeSkillName(r.URL.Query().Get("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	raw, err := skillregistry.DownloadSkill(r.Context(), name)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	var pack skillpack.SignedPack
	if err := json.Unmarshal(raw, &pack); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bukan signed pack: " + err.Error()})
		return
	}
	dir := brain.DynamicSkillsDir()
	if dir == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "skills dir unresolved"})
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "mkdir: " + err.Error()})
		return
	}
	cleanDir := filepath.Clean(dir)
	imported := 0
	results := make([]map[string]any, 0, len(pack.Skills))
	overwrite := r.URL.Query().Get("overwrite") == "1"
	for _, s := range pack.Skills {
		sn := sanitizeSkillName(s.Name)
		v := map[string]any{"name": sn}
		sigValid := mesh.VerifyData(pack.AuthorPubkey, []byte(s.Content), s.Sig) // gerbang #1 verify
		flags := skillpack.VerifyContent(s.Content)                              // gerbang #2 verify
		switch {
		case sn == "":
			v["status"], v["reason"] = "rejected", "nama invalid"
		case !sigValid:
			v["status"], v["reason"] = "rejected", "signature invalid (provenance/integritas gagal)"
		case len(flags) > 0:
			v["status"], v["reason"] = "rejected", "unsafe content: "+strings.Join(flags, "; ")
		case !strings.HasPrefix(strings.TrimSpace(s.Content), "---"):
			v["status"], v["reason"] = "rejected", "bukan SKILL.md (frontmatter '---' wajib)"
		default:
			path := filepath.Join(dir, sn+".md")
			if !strings.HasPrefix(filepath.Clean(path), cleanDir+string(os.PathSeparator)) {
				v["status"], v["reason"] = "rejected", "path traversal"
			} else if _, statErr := os.Stat(path); statErr == nil && !overwrite {
				v["status"], v["reason"] = "skipped", "sudah ada (pakai ?overwrite=1)"
			} else if werr := os.WriteFile(path, []byte(s.Content), 0o644); werr != nil {
				v["status"], v["reason"] = "error", werr.Error()
			} else {
				v["status"] = "imported"
				imported++
			}
		}
		results = append(results, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported, "author_pubkey": pack.AuthorPubkey,
		"total": len(pack.Skills), "skills": results,
	})
}

// skillRegistryPublishHandler — POST /api/skills/registry/publish?skill=
func skillRegistryPublishHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if !loopbackOnly(w, r) {
		return
	}
	if !skillregistry.HasToken() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "FLOWORK_GITHUB_TOKEN belum di-set — gak bisa publish"})
		return
	}
	skill := sanitizeSkillName(r.URL.Query().Get("skill"))
	if skill == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "skill required"})
		return
	}
	content, ok := readOneSkill(brain.DynamicSkillsDir(), skill)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "skill tidak ditemukan: " + skill})
		return
	}
	db, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if pub, reason := skillpack.CanPublish(db, skill, content); !pub { // gerbang #3
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "karma-gate: " + reason})
		return
	}
	sig, pubkey, serr := mesh.SignData(db, []byte(content)) // gerbang #1
	if serr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "sign: " + serr.Error()})
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	pack := skillpack.SignedPack{Kind: "fwskill-signed", Version: 1, AuthorPubkey: pubkey,
		SignedAt: now, Source: "flowork", Skills: []skillpack.SignedSkill{{Name: skill, Content: content, Sig: sig}}}
	fwskill, _ := json.Marshal(pack)
	sum := sha256.Sum256(fwskill)
	entry := skillregistry.IndexEntry{Name: skill, AuthorPubkey: pubkey, Sha256: hex.EncodeToString(sum[:]),
		Description: frontmatterDesc(content), Sig: sig}
	if err := skillregistry.Publish(r.Context(), fwskill, entry, now); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "publish: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "published": skill, "repo": skillregistry.Repo(), "sha256": entry.Sha256,
	})
}
