// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SkillListResp struct {
	Items []SkillSummary `json:"items"`
	Count int            `json:"count"`
	Total int            `json:"total"`
}

type SkillDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	var out SkillListResp
	if uerr := json.Unmarshal(body, &out); uerr != nil {
		return SkillListResp{}, fmt.Errorf("decode: %w", uerr)
	}
	return out, nil
}

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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	var out SkillDoc
	if uerr := json.Unmarshal(body, &out); uerr != nil {
		return SkillDoc{}, fmt.Errorf("decode: %w", uerr)
	}
	return out, nil
}
