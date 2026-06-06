// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 7 phase 2 (PullSkill: list + get). API stable: ListSkills,
//   GetSkill. Mirror Router /api/brain/skills/list + /api/brain/skills/get
//   yang locked sejak 2026-05-29. Phase 3 (skill metadata cache, prefetch,
//   ETag) → tambah file baru, JANGAN modify ini.
//
// skills.go — Section 7 phase 2: skill catalog retrieve dari Router brain.
//
// Pattern mirror brain_search.go (Section 11 phase 1e). Reuse locked Client
// (routerclient.go) — capability whitelist + timeout dari ctor New.
//
// Source: Flowork_Agent/roadmap.md Section 7 phase 2.

package routerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// SkillSummary — minimal payload dari Router /api/brain/skills/list.
// Mirror Router's SkillSummary shape (anti over-prompt: name + description
// only, JANGAN load body — caller fetch on-demand via GetSkill).
type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillListResp — full response dari Router list endpoint.
type SkillListResp struct {
	Items []SkillSummary `json:"items"`
	Count int            `json:"count"`
	Total int            `json:"total"`
}

// SkillDoc — full skill detail dari Router /api/brain/skills/get.
// Mirror Router's brain.SkillDoc.
type SkillDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// ListSkills — GET /api/brain/skills/list?search=&limit=
//
// Router cap limit ke 10 (anti over-prompt). Caller pass limit > 10 → router
// silently clamp. search empty → ranked by embed order.
//
// Return error kalau status >= 400 atau decode gagal.
func (c *Client) ListSkills(ctx context.Context, search string, limit int) (SkillListResp, error) {
	if c == nil {
		return SkillListResp{}, fmt.Errorf("router client nil")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 10 {
		limit = 10
	}

	q := url.Values{}
	if search != "" {
		q.Set("search", search)
	}
	q.Set("limit", strconv.Itoa(limit))
	u := c.BaseURL + "/api/brain/skills/list?" + q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return SkillListResp{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return SkillListResp{}, fmt.Errorf("list skills: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return SkillListResp{}, fmt.Errorf("router status %d", resp.StatusCode)
	}
	// 10 summary × ~512B = 5KB. Cap 128KB generous.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	var out SkillListResp
	if uerr := json.Unmarshal(body, &out); uerr != nil {
		return SkillListResp{}, fmt.Errorf("decode: %w", uerr)
	}
	return out, nil
}

// GetSkill — GET /api/brain/skills/get?id=<name>
//
// Return full SkillDoc (incl. body markdown). Caller validate skill name
// non-empty.
func (c *Client) GetSkill(ctx context.Context, name string) (SkillDoc, error) {
	if c == nil {
		return SkillDoc{}, fmt.Errorf("router client nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return SkillDoc{}, fmt.Errorf("skill name required")
	}

	q := url.Values{}
	q.Set("id", name)
	u := c.BaseURL + "/api/brain/skills/get?" + q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return SkillDoc{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return SkillDoc{}, fmt.Errorf("get skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return SkillDoc{}, fmt.Errorf("skill not found: %s", name)
	}
	if resp.StatusCode >= 400 {
		return SkillDoc{}, fmt.Errorf("router status %d", resp.StatusCode)
	}
	// Skill body max ~64KB markdown (per brain.SkillDoc constraint), cap 256KB.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	var out SkillDoc
	if uerr := json.Unmarshal(body, &out); uerr != nil {
		return SkillDoc{}, fmt.Errorf("decode: %w", uerr)
	}
	return out, nil
}
