// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/floworkdb"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/loket"
	"flowork-gui/internal/routerclient"
)

func init() {
	budget := 240 * time.Second
	if v := strings.TrimSpace(os.Getenv("FLOWORK_FANOUT_BUDGET")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			budget = d
		}
	}
	loket.FanoutStrategy = func(targets []string, invoke func(string) (json.RawMessage, error)) []loket.FanoutBroadcastReply {
		return loket.ParallelFanout(budget, targets, invoke)
	}
}

func wireLoket(host *kernelhost.Host) *loket.Service {
	deps := loket.Deps{

		StorePath: func(module string) (string, error) {
			staged := filepath.Join(loader.AgentsDir(), module+".fwagent")
			dbPath := agentdb.Resolve(module, staged)
			return filepath.Join(filepath.Dir(dbPath), "loket.db"), nil
		},

		ModuleDir: func(module string) (string, error) {
			return filepath.Join(loader.AgentsDir(), module+".fwagent"), nil
		},

		IsPrimary: agentmgr.IsPrimaryAgent,

		Send: func(ctx context.Context, target string, msg loket.Message) error {
			_, err := invokeLoketModule(ctx, host, target, msg)
			return err
		},
		Invoke: func(ctx context.Context, target string, msg loket.Message) (json.RawMessage, error) {
			return invokeLoketModule(ctx, host, target, msg)
		},

		Modules: func() []loket.ModuleInfo {
			if host == nil {
				return nil
			}
			ids := host.AgentIDs()
			out := make([]loket.ModuleInfo, 0, len(ids))
			for _, id := range ids {
				info := loket.ModuleInfo{ID: id, Kind: "agent"}
				raw, err := os.ReadFile(filepath.Join(loader.AgentsDir(), id+".fwagent", "loket.json"))
				if err == nil {
					if m, perr := loket.ParseManifest(raw); perr == nil {
						info.Kind = string(m.Kind)
						info.Provides = m.Provides
					}
				}
				out = append(out, info)
			}
			return out
		},

		NotifyOwner: func(ctx context.Context, text string) error {
			return notifyOwnerTelegram(ctx, text)
		},
	}
	svc := loket.NewService(deps, os.Getenv("FLOWORK_LOOPBACK_SECRET"))

	_ = svc.Kernel.Register("llm.complete", llmCompleteProvider)
	_ = svc.Kernel.Register("brain.shared.search", brainSharedSearchProvider)
	_ = svc.Kernel.Register("brain.shared.promote", brainSharedPromoteProvider)

	_ = svc.Kernel.Register("tool.specs", toolSpecsProvider)
	_ = svc.Kernel.Register("tool.run", toolRunProvider)
	_ = svc.Kernel.Register("slash.run", slashRunProvider)

	svc.Kernel.SetRateLimit(6000)
	return svc
}

func invokeLoketModule(ctx context.Context, host *kernelhost.Host, target string, msg loket.Message) (json.RawMessage, error) {
	if host == nil || host.Runtime == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	inst := host.Runtime.Get(target)
	if inst == nil {
		return nil, fmt.Errorf("target module %q not loaded", target)
	}
	body, _ := json.Marshal(msg)
	out, err := inst.Call(ctx, "handle", body)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []byte("{}")
	}
	return json.RawMessage(out), nil
}

func llmCompleteProvider(ctx context.Context, _ string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Messages          json.RawMessage `json:"messages"`
		Model             string          `json:"model"`
		Tools             json.RawMessage `json:"tools,omitempty"`
		ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
		ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if len(a.Messages) == 0 {
		return nil, fmt.Errorf("llm.complete: messages required")
	}
	if a.Model == "" {

		a.Model = floworkdb.DefaultModelShared()
		if a.Model == "" {
			a.Model = "claude-haiku-4-5"
		}
	}
	reqMap := map[string]any{"model": a.Model, "messages": a.Messages}
	if len(a.Tools) > 0 {
		reqMap["tools"] = a.Tools

		if a.ParallelToolCalls != nil {
			reqMap["parallel_tool_calls"] = *a.ParallelToolCalls
		} else {
			reqMap["parallel_tool_calls"] = false
		}
	}
	if len(a.ToolChoice) > 0 {
		reqMap["tool_choice"] = a.ToolChoice
	}
	reqBody, _ := json.Marshal(reqMap)

	url := routerclient.New(os.Getenv("ROUTER_DEFAULT_URL")).BaseURL + "/v1/chat/completions"

	cctx, cancel := context.WithTimeout(ctx, 240*time.Second)
	defer cancel()

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(cctx, http.MethodPost, url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, lastErr = http.DefaultClient.Do(req)
		if lastErr == nil && resp.StatusCode < 500 {
			break
		}
		if resp != nil {
			resp.Body.Close()
			resp = nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("llm router: %w", lastErr)
	}
	if resp == nil {
		return nil, fmt.Errorf("llm router: no response after retries")
	}
	defer resp.Body.Close()
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string          `json:"content"`
				ToolCalls json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("llm decode: %w", err)
	}
	if parsed.Error != nil {
		eb, _ := json.Marshal(parsed.Error)
		return nil, fmt.Errorf("llm: %s", string(eb))
	}
	out := map[string]any{"content": ""}
	if len(parsed.Choices) > 0 {
		out["content"] = parsed.Choices[0].Message.Content
		if len(parsed.Choices[0].Message.ToolCalls) > 0 {
			out["tool_calls"] = parsed.Choices[0].Message.ToolCalls
		}
	}
	return json.Marshal(out)
}

func toolSpecsProvider(_ context.Context, module string, _ json.RawMessage) (json.RawMessage, error) {
	req := httptest.NewRequest(http.MethodGet, "/api/agents/tools/specs?id="+module, nil)
	stampCaller(req, module)
	rec := httptest.NewRecorder()
	agentmgr.ToolSpecsHandler(rec, req)
	return rec.Body.Bytes(), nil
}

func toolRunProvider(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.Name == "" {
		return nil, fmt.Errorf("tool.run: name required")
	}
	if len(a.Args) == 0 {
		a.Args = json.RawMessage("{}")
	}
	body, _ := json.Marshal(map[string]any{"tool_name": a.Name, "args": a.Args})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/tools/run?id="+module, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	stampCaller(req, module)
	rec := httptest.NewRecorder()
	agentmgr.ToolRunHandler(rec, req)

	return redactToolSecrets(rec.Body.Bytes()), nil
}

var secretRe = regexp.MustCompile(`(ghp_|gho_|ghs_|ghu_|github_pat_|sk-|xox[baprs]-)[A-Za-z0-9_\-]{16,}|AKIA[0-9A-Z]{16}|[0-9]{8,10}:[A-Za-z0-9_-]{35}`)

func redactToolSecrets(b []byte) []byte {
	s := string(b)
	if v := strings.TrimSpace(os.Getenv("FLOWORK_LOOPBACK_SECRET")); len(v) >= 8 {
		s = strings.ReplaceAll(s, v, "[REDACTED]")
	}
	s = secretRe.ReplaceAllString(s, "[REDACTED]")
	return []byte(s)
}

func stampCaller(req *http.Request, module string) {
	if secret := os.Getenv("FLOWORK_LOOPBACK_SECRET"); secret != "" {
		req.Header.Set("X-Flowork-Secret", secret)
		req.Header.Set("X-Flowork-Caller", module)
	}
}

func slashRunProvider(_ context.Context, module string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Text) == "" {
		return nil, fmt.Errorf("slash.run: text required")
	}
	body, _ := json.Marshal(map[string]any{"text": a.Text, "caller": module})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/slash/run?id="+module, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	stampCaller(req, module)
	rec := httptest.NewRecorder()
	agentmgr.SlashRunHandler(rec, req)
	return redactToolSecrets(rec.Body.Bytes()), nil
}

func brainSharedSearchProvider(ctx context.Context, _ string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Query string `json:"query"`
		K     int    `json:"k"`
	}
	_ = json.Unmarshal(args, &a)
	if a.K <= 0 {
		a.K = 5
	}
	resp, err := routerclient.New("").SearchBrain(ctx, a.Query, a.K)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"hits": resp.Hits, "count": resp.Count})
}

func brainSharedPromoteProvider(ctx context.Context, _ string, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Content string `json:"content"`
		Wing    string `json:"wing"`
		Room    string `json:"room"`
		MemType string `json:"mem_type"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, err
	}
	if a.Content == "" {
		return nil, fmt.Errorf("brain.shared.promote: content required")
	}
	resp, err := routerclient.New("").PromoteDrawer(ctx, routerclient.PromoteDrawerReq{
		Content: a.Content, Wing: a.Wing, Room: a.Room, MemType: a.MemType,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"id": resp.ID, "added": resp.Added})
}
