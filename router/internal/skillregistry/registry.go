// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval. Owner: Aola Sahidin (Mr.Dev).
// Locked 2026-06-17 · P2 fase-2b transport registry komunitas, owner-approved (owner kasih
//   full izin GitHub API + minta bikin repo flowork-os/flowork-skills). E2E tested live:
//   publish→browse(fresh)→pull→verify round-trip. Browse/pull pakai contents API (anti-cache).
//
// Package skillregistry — P2 A2 fase-2b: transport ke registry skill komunitas (GitHub).
//
// BROWSE/PULL = baca PUBLIK (raw.githubusercontent, TANPA token) → konsumsi registry aman
// tanpa kredensial. PUBLISH = tulis (GitHub contents API, BUTUH token) → kontribusi.
//
// Model kepercayaan (3 gerbang) tetap di-enforce di handler:
//   - publish: skill WAJIB lolos karma-gate (kebukti bagus lokal) + di-sign (provenance).
//   - pull:    skill di-verify (signature + content gate) SEBELUM import. Registry = untrusted.
//
// Repo default flowork-os/flowork-skills (override env FLOWORK_SKILL_REGISTRY="owner/repo").
// Token publish dari env FLOWORK_GITHUB_TOKEN (owner-local; GUI/secrets nyusul).
package skillregistry

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultRepo = "flowork-os/flowork-skills"
	branch      = "main"
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

// IndexEntry — satu skill di katalog registry.
type IndexEntry struct {
	Name         string `json:"name"`
	AuthorPubkey string `json:"author_pubkey"`
	Sha256       string `json:"sha256"`
	Description  string `json:"description,omitempty"`
	Sig          string `json:"sig"`
	UpdatedAt    string `json:"updated_at"`
}

// Index — isi registry/index.json.
type Index struct {
	RegistryVersion int          `json:"registry_version"`
	UpdatedAt       string       `json:"updated_at"`
	Skills          []IndexEntry `json:"skills"`
}

// Repo mengembalikan "owner/repo" (env override atau default).
func Repo() string {
	if r := strings.TrimSpace(os.Getenv("FLOWORK_SKILL_REGISTRY")); r != "" {
		return r
	}
	return defaultRepo
}

func token() string { return strings.TrimSpace(os.Getenv("FLOWORK_GITHUB_TOKEN")) }

func apiContentsURL(path string) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", Repo(), path)
}

func skillPath(name string) string { return "skills/" + name + "/" + name + ".fwskill" }

// getContentsRaw GET file via GitHub contents API dgn Accept: raw → isi FRESH. (raw.github
// usercontent CDN ke-cache ~menit → publish lalu browse langsung gak keliatan; API contents
// fresh.) Token opsional (longgarin rate-limit; public repo bisa tanpa). status 404 = belum ada.
func getContentsRaw(ctx context.Context, path string) ([]byte, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiContentsURL(path)+"?ref="+branch, nil)
	req.Header.Set("Accept", "application/vnd.github.raw")
	if t := token(); t != "" {
		req.Header.Set("Authorization", "token "+t)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return body, resp.StatusCode, nil
}

// FetchIndex baca registry/index.json (fresh via contents API). Index kosong (bukan error)
// kalau registry baru / 404.
func FetchIndex(ctx context.Context) (Index, error) {
	var idx Index
	body, code, err := getContentsRaw(ctx, "registry/index.json")
	if err != nil {
		return idx, fmt.Errorf("fetch index: %w", err)
	}
	if code == http.StatusNotFound {
		return Index{RegistryVersion: 1, Skills: []IndexEntry{}}, nil
	}
	if code != http.StatusOK {
		return idx, fmt.Errorf("fetch index: HTTP %d", code)
	}
	if err := json.Unmarshal(body, &idx); err != nil {
		return idx, fmt.Errorf("parse index: %w", err)
	}
	return idx, nil
}

// DownloadSkill baca .fwskill by-name (fresh via contents API).
func DownloadSkill(ctx context.Context, name string) ([]byte, error) {
	body, code, err := getContentsRaw(ctx, skillPath(name))
	if err != nil {
		return nil, fmt.Errorf("download %q: %w", name, err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("download %q: HTTP %d", name, code)
	}
	return body, nil
}

// getFileSha ambil sha file existing (kosong kalau 404) — buat update contents API.
func getFileSha(ctx context.Context, tok, path string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiContentsURL(path)+"?ref="+branch, nil)
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get sha %s: HTTP %d", path, resp.StatusCode)
	}
	var r struct {
		Sha string `json:"sha"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	_ = json.Unmarshal(body, &r)
	return r.Sha, nil
}

// putFile tulis/replace file via contents API (butuh token).
func putFile(ctx context.Context, tok, path string, content []byte, message string) error {
	sha, err := getFileSha(ctx, tok, path)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, apiContentsURL(path), bytes.NewReader(buf))
	req.Header.Set("Authorization", "token "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("put %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return fmt.Errorf("put %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// HasToken = true kalau token publish tersedia (env).
func HasToken() bool { return token() != "" }

// Publish push satu skill ber-tanda-tangan ke registry + update index (butuh token).
//   1. PUT skills/<name>/<name>.fwskill
//   2. GET+merge registry/index.json (upsert entry by name) → PUT
// fwskill = signed pack bytes. entry = metadata katalog (sudah diisi caller: sig, pubkey, sha256).
func Publish(ctx context.Context, fwskill []byte, entry IndexEntry, nowRFC string) error {
	tok := token()
	if tok == "" {
		return fmt.Errorf("FLOWORK_GITHUB_TOKEN belum di-set — gak bisa publish")
	}
	if entry.Name == "" {
		return fmt.Errorf("entry.name kosong")
	}
	// 1) skill file
	if err := putFile(ctx, tok, skillPath(entry.Name), fwskill,
		"registry: publish skill "+entry.Name); err != nil {
		return err
	}
	// 2) merge index
	idx, err := FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("read index for merge: %w", err)
	}
	if idx.RegistryVersion == 0 {
		idx.RegistryVersion = 1
	}
	entry.UpdatedAt = nowRFC
	upserted := false
	for i := range idx.Skills {
		if idx.Skills[i].Name == entry.Name {
			idx.Skills[i] = entry
			upserted = true
			break
		}
	}
	if !upserted {
		idx.Skills = append(idx.Skills, entry)
	}
	idx.UpdatedAt = nowRFC
	buf, _ := json.MarshalIndent(idx, "", "  ")
	if err := putFile(ctx, tok, "registry/index.json", buf,
		"registry: index += "+entry.Name); err != nil {
		return err
	}
	return nil
}
