// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-16 (autonomous sprint A2 fase-1).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner).
// 2026-06-17 (owner-approved P2 fase-2a gerbang #2): import sekarang lewat
//   skillgate.Verify (scan KONTEN body — syscall berbahaya + prompt-injection), bukan
//   cuma cek struktur. Skill tak-aman ditolak sebelum ditulis ke disk.
//
// skills_exchange.go — A2 BURSA MEMETIK fase-1: export/import skill pack (.fwskill) antar-instance.
//
// Skill (yg shareable) = file SKILL.md di dynamic-skills dir (architectSkillsDir, sama dgn yg router
// brain inject by-keyword). Fase-1 = EXPORT/IMPORT MANUAL (.fwskill = JSON bundle). Fase-2 (registry
// komunitas + karma + verifier konten) = nanti (lihat roadmap_asi A2).
//
// KEAMANAN (skill dari luar = UNTRUSTED): import OWNER-GATED (auth middleware, sama dgn endpoint admin
// lain) + verifier STRUKTURAL — nama di-sanitize anti path-traversal (skillNameRe, sama dgn authorSkill),
// ukuran cap 64KB/skill, frontmatter '---' wajib, resolved-path WAJIB di dalam skills dir, default GAK
// nimpa skill yg udah ada (butuh ?overwrite=1). CATATAN: skill body = teks yg masuk prompt → vetting
// NIAT konten (anti prompt-injection) = tanggung jawab owner pas approve; fase-1 jamin STRUKTUR aman.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/skillgate"
)

type fwskillEntry struct {
	Name    string `json:"name"`
	Content string `json:"content"` // full SKILL.md (frontmatter + body)
}

type fwskillPack struct {
	Kind       string         `json:"kind"`    // "fwskill"
	Version    int            `json:"version"` // 1
	ExportedAt string         `json:"exported_at"`
	Source     string         `json:"source"`
	Skills     []fwskillEntry `json:"skills"`
}

const (
	maxSkillBytes   = 64 * 1024 // 64KB per skill (SKILL.md = dokumen fokus, bukan blob)
	maxSkillsInPack = 500
)

// skillsExportHandler — GET /api/skills/export → bundle semua SKILL.md di dynamic-skills dir jadi
// .fwskill pack (JSON) yg bisa di-download. Owner-gated (auth middleware).
func skillsExportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET only"})
		return
	}
	dir := architectSkillsDir()
	pack := fwskillPack{Kind: "fwskill", Version: 1, ExportedAt: time.Now().UTC().Format(time.RFC3339), Source: "flowork"}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
				continue
			}
			b, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
			if rerr != nil || len(b) == 0 || len(b) > maxSkillBytes {
				continue
			}
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			pack.Skills = append(pack.Skills, fwskillEntry{Name: name, Content: string(b)})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=flowork-skills.fwskill")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"pack": pack, "count": len(pack.Skills)})
}

// skillsImportHandler — POST /api/skills/import {kind,skills:[{name,content}]} → verify tiap skill +
// tulis ke dynamic-skills dir. Owner-gated. Default GAK nimpa (butuh ?overwrite=1). Balik verdict per-skill.
func skillsImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		tfWriteJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	dir := architectSkillsDir()
	if dir == "" {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "skills dir unresolved"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 32<<20)) // 32MB cap seluruh pack
	if err != nil {
		tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "read: " + err.Error()})
		return
	}
	// Terima dua bentuk: {pack:{...}} (mirror export) atau {kind,skills} langsung.
	var wrap struct {
		Pack *fwskillPack `json:"pack"`
	}
	var pack fwskillPack
	if json.Unmarshal(raw, &wrap) == nil && wrap.Pack != nil {
		pack = *wrap.Pack
	} else if err := json.Unmarshal(raw, &pack); err != nil {
		tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid .fwskill JSON: " + err.Error()})
		return
	}
	if pack.Kind != "" && pack.Kind != "fwskill" {
		tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "bukan .fwskill pack (kind=" + pack.Kind + ")"})
		return
	}
	if len(pack.Skills) > maxSkillsInPack {
		tfWriteJSON(w, http.StatusBadRequest, map[string]any{"error": "kebanyakan skill di pack"})
		return
	}
	overwrite := r.URL.Query().Get("overwrite") == "1"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tfWriteJSON(w, http.StatusInternalServerError, map[string]any{"error": "mkdir: " + err.Error()})
		return
	}
	cleanDir := filepath.Clean(dir)
	imported := 0
	results := make([]map[string]any, 0, len(pack.Skills))
	for _, s := range pack.Skills {
		// VERIFIER anti-traversal: re-sanitize ke charset authorSkill → "../../x" mustahil escape.
		name := strings.Trim(skillNameRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(s.Name)), "-"), "-")
		v := map[string]any{"name": name}
		switch {
		case name == "":
			v["status"], v["reason"] = "rejected", "nama kosong/invalid"
		case strings.TrimSpace(s.Content) == "" || len(s.Content) > maxSkillBytes:
			v["status"], v["reason"] = "rejected", "kosong atau >64KB"
		case !strings.HasPrefix(strings.TrimSpace(s.Content), "---"):
			v["status"], v["reason"] = "rejected", "bukan SKILL.md (frontmatter '---' wajib)"
		case len(skillgate.Verify(s.Content)) > 0:
			// Gerbang #2 (P2 fase-2a): skill UNTRUSTED → scan KONTEN body untuk pola
			// syscall berbahaya + prompt-injection sebelum boleh masuk (skill = teks
			// yang di-inject ke prompt tiap turn). Mirror anti-poison gate skill_author.
			v["status"], v["reason"] = "rejected", "unsafe content: "+strings.Join(skillgate.Verify(s.Content), "; ")
		default:
			path := filepath.Join(dir, name+".md")
			if !strings.HasPrefix(filepath.Clean(path), cleanDir+string(os.PathSeparator)) {
				v["status"], v["reason"] = "rejected", "path traversal"
			} else if _, statErr := os.Stat(path); statErr == nil && !overwrite {
				v["status"], v["reason"] = "skipped", "sudah ada (pakai ?overwrite=1 buat timpa)"
			} else if werr := os.WriteFile(path, []byte(s.Content), 0o644); werr != nil {
				v["status"], v["reason"] = "error", werr.Error()
			} else {
				v["status"] = "imported"
				imported++
			}
		}
		results = append(results, v)
	}
	tfWriteJSON(w, 0, map[string]any{"imported": imported, "total": len(pack.Skills), "skills": results,
		"note": "skill di-inject router brain by-keyword. Owner WAJIB review isi skill (anti prompt-injection) — fase-1 cuma jamin struktur aman."})
}
