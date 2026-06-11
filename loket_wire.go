// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Locked at: 2026-06-11
// Reason: Loket wiring (package main, NOT frozen-kernel) — carries the LLM
//   transport (llmCompleteProvider) and the loopback-secret guard, so it is
//   security-relevant. Audit pass — loopback secret from os.Getenv, tool-result
//   secret redaction, retry on 5xx only.
// 2026-06-11 OWNER-APPROVED: llm.complete now resolves the base URL from
//   Settings → Default Router (ROUTER_DEFAULT_URL) and the model from
//   FLOWORK_LLM_MODEL when the caller pins neither. Read HOST-SIDE only.
//   routerclient.New enforces the localhost host-whitelist → a stray/external
//   router URL safely falls back to the built-in default, never an exfil vector.
//
// loket_wire.go — wire the new microkernel ("loket") into the running process.
//
// ADDITIVE + non-breaking: this adds ONE endpoint and ONE new Kernel instance
// beside the existing kernel. Legacy agents keep their old code paths; only
// loket-native modules use /api/kernel/call. This is the safe "build alongside,
// migrate later" path — the old system stays alive until the new one is proven.
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
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/loket"
	"flowork-gui/internal/routerclient"
)

// init registers the PLUG-AND-PLAY parallel fan-out for bus.broadcast (P5). The
// runtime instantiates a FRESH module per Call (unique name via atomic counter), so
// invoking distinct colony members concurrently is safe — a council/group fans out in
// parallel instead of one-at-a-time. It is also BOUNDED: a member that hangs is capped
// by a budget and reported as a timeout while the rest still complete (the coordinator's
// "stop / collect-partial" lifecycle). Registered here (non-frozen) so the frozen kernel
// (internal/loket/providers.go) never needs editing again to change coordination.
func init() {
	budget := 120 * time.Second
	if v := strings.TrimSpace(os.Getenv("FLOWORK_FANOUT_BUDGET")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			budget = d
		}
	}
	loket.FanoutStrategy = func(targets []string, invoke func(string) (json.RawMessage, error)) []loket.FanoutBroadcastReply {
		return loket.ParallelFanout(budget, targets, invoke)
	}
}

// wireLoket builds the loket Service with real, host-backed dependencies.
func wireLoket(host *kernelhost.Host) *loket.Service {
	deps := loket.Deps{
		// StorePath: each module's own loket store lives beside its workspace,
		// in its own folder — that per-folder mapping IS the storage isolation.
		StorePath: func(module string) (string, error) {
			staged := filepath.Join(loader.AgentsDir(), module+".fwagent")
			dbPath := agentdb.Resolve(module, staged)
			return filepath.Join(filepath.Dir(dbPath), "loket.db"), nil
		},
		// ModuleDir = the module's own folder, where its loket.json (the manifest
		// declaring which capabilities it consumes) lives.
		ModuleDir: func(module string) (string, error) {
			return filepath.Join(loader.AgentsDir(), module+".fwagent"), nil
		},
		// IsPrimary is the AUTHORITATIVE tier answer — the owner-controlled allowlist
		// (agentmgr.primaryAgents), NOT the agent's self-declared manifest tier. This
		// is what stops a module from self-promoting to the tier-gated 5M corpus by
		// writing tier:"primary" in its own loket.json.
		IsPrimary: agentmgr.IsPrimaryAgent,
		// Send / Invoke route a message to a target module's "handle" export via
		// the existing wasm runtime. The kernel already stamped msg.Source.
		Send: func(ctx context.Context, target string, msg loket.Message) error {
			_, err := invokeLoketModule(ctx, host, target, msg)
			return err
		},
		Invoke: func(ctx context.Context, target string, msg loket.Message) (json.RawMessage, error) {
			return invokeLoketModule(ctx, host, target, msg)
		},
		// Modules lists loaded modules for discovery (registry.*). Kind + provides
		// come from each module's own loket.json (manifest-driven, no hardcoding).
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
		// NotifyOwner routes bus.send(to:"owner") to the owner's Telegram (the
		// existing owner-notify path). "owner" stays a logical address.
		NotifyOwner: func(ctx context.Context, text string) error {
			return notifyOwnerTelegram(ctx, text)
		},
	}
	svc := loket.NewService(deps, os.Getenv("FLOWORK_LOOPBACK_SECRET"))

	// Service-provided caps (SourceService) — swappable. The LLM is a SERVICE,
	// not part of the kernel: point it at a local model and the kernel never
	// changes. That is sovereignty (the "pasukan semut" running on local models).
	_ = svc.Kernel.Register("llm.complete", llmCompleteProvider)
	_ = svc.Kernel.Register("brain.shared.search", brainSharedSearchProvider)
	_ = svc.Kernel.Register("brain.shared.promote", brainSharedPromoteProvider)
	// Tool bridge: reach the engine's existing tool surface by name. Routing is
	// DATA — these point at the in-engine registry today, a folder module tomorrow
	// (§D), without the kernel ever changing.
	_ = svc.Kernel.Register("tool.specs", toolSpecsProvider)
	_ = svc.Kernel.Register("tool.run", toolRunProvider)
	_ = svc.Kernel.Register("slash.run", slashRunProvider)

	// Generous per-module safety cap (100 calls/sec). Normal work — even a group
	// fanning out to its members — stays far below this; it only contains a
	// runaway module stuck in a loop (e.g. spending money on llm.complete).
	svc.Kernel.SetRateLimit(6000)
	return svc
}

// invokeLoketModule delivers a loket Message to a target module's "handle"
// export through the existing runtime and returns its raw reply.
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

// llmCompleteProvider calls the router's OpenAI-compatible endpoint. It is the
// only place that knows the LLM transport, so swapping providers is a one-line
// change here, never in the kernel. Args: {messages:[{role,content}], model?}.
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
		// Caller didn't pin a model → fall back to the Settings → Default Model
		// (FLOWORK_LLM_MODEL), then to a SMALL model — the ant ethos.
		a.Model = strings.TrimSpace(os.Getenv("FLOWORK_LLM_MODEL"))
		if a.Model == "" {
			a.Model = "claude-haiku-4-5"
		}
	}
	reqMap := map[string]any{"model": a.Model, "messages": a.Messages}
	if len(a.Tools) > 0 {
		reqMap["tools"] = a.Tools
		// Force sequential tool calls: the router's subscription path mistranslates
		// parallel tool_results (>1 per message) into an anthropic 400. One tool per
		// turn keeps it correct. Honour an explicit override if the caller sent one.
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
	// Base = Settings → Default Router URL (ROUTER_DEFAULT_URL) when set, else the
	// built-in default. This is read HOST-SIDE only: agents reach the LLM through
	// this provider (the one place that knows the transport), so the URL never
	// needs to cross into a sandboxed agent's env. routerclient.New validates the
	// host (localhost whitelist), so a stray/external value safely falls back —
	// never an exfil vector.
	url := routerclient.New(os.Getenv("ROUTER_DEFAULT_URL")).BaseURL + "/v1/chat/completions"

	cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	// Light retry on transient 5xx / network blips — the same failure the legacy
	// agent rode out (router 502 "all providers failed", anthropic 529 overload).
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

// toolSpecsProvider lists the OpenAI function schemas the engine exposes to the
// LLM for this module — the bridge to the existing tool surface (tool.specs). It
// calls the engine's own specs handler IN-PROCESS (no network, no auth hop), so
// every selection / anti-over-prompt rule the handler enforces still holds.
func toolSpecsProvider(_ context.Context, module string, _ json.RawMessage) (json.RawMessage, error) {
	req := httptest.NewRequest(http.MethodGet, "/api/agents/tools/specs?id="+module, nil)
	stampCaller(req, module)
	rec := httptest.NewRecorder()
	agentmgr.ToolSpecsHandler(rec, req)
	return rec.Body.Bytes(), nil
}

// toolRunProvider executes ONE registered tool by name on the module's behalf
// (tool.run). It calls the engine's own run handler in-process, so the per-tool
// sandbox + consent + tier gates (SandboxRunV3) run exactly as they do for a
// legacy agent — a second lock on top of the loket grant, never a bypass.
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
	// Redact secrets from the tool result BEFORE it flows back to the agent → LLM
	// → external model. A tool that reads a file/env could otherwise leak a token.
	return redactToolSecrets(rec.Body.Bytes()), nil
}

// secretRe matches common credential shapes (GitHub/OpenAI/Slack tokens, AWS keys,
// Telegram bot tokens) so they never ride a tool result into the LLM context.
var secretRe = regexp.MustCompile(`(ghp_|gho_|ghs_|ghu_|github_pat_|sk-|xox[baprs]-)[A-Za-z0-9_\-]{16,}|AKIA[0-9A-Z]{16}|[0-9]{8,10}:[A-Za-z0-9_-]{35}`)

// redactToolSecrets scrubs a tool result of (a) the kernel's own loopback secret
// (exact value from env — leaking it would let a module forge calls) and (b)
// credential-shaped strings. Conservative: it never touches ordinary output.
func redactToolSecrets(b []byte) []byte {
	s := string(b)
	if v := strings.TrimSpace(os.Getenv("FLOWORK_LOOPBACK_SECRET")); len(v) >= 8 {
		s = strings.ReplaceAll(s, v, "[REDACTED]")
	}
	s = secretRe.ReplaceAllString(s, "[REDACTED]")
	return []byte(s)
}

// stampCaller marks an in-process bridge request with the module's VERIFIED id
// the same way the host marks a guest's outbound call, so the engine handlers
// bind execution to the real caller (anti-spoof) instead of a ?id= guess.
func stampCaller(req *http.Request, module string) {
	if secret := os.Getenv("FLOWORK_LOOPBACK_SECRET"); secret != "" {
		req.Header.Set("X-Flowork-Secret", secret)
		req.Header.Set("X-Flowork-Caller", module)
	}
}

// slashRunProvider dispatches a slash command on the module's behalf (slash.run),
// bridging to the engine's slash registry in-process — same pattern as tool.run.
// The result is secret-redacted before it returns to the agent.
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

// brainSharedSearchProvider serves brain.shared.search via the router's 5M
// corpus. Tier-gating (primary-only) is enforced upstream by the dispatcher's
// grant check; this provider just performs the search.
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

// brainSharedPromoteProvider contributes one drawer to the 5M shared corpus
// (brain.shared.promote). Tier-gating (primary-only) is enforced upstream by the
// dispatcher's grant check; this provider just performs the promote via the router.
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
