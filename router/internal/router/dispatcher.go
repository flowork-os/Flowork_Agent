// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: Audit pass — Router dispatcher.
// 2026-06-13 OWNER-APPROVED (audit→review→test→lock): the Claude subscription branch now uses
//   creds.LoadValid (auto-refresh of an expired OAuth token via refresh_token grant) instead of a
//   hard "expired — re-login Claude Code" error, so Claude survives unattended on Android / USB with
//   no Claude Code. Single line change; refresh logic lives in the non-frozen internal/creds package
//   and is mock-server unit-tested. Not hash-frozen (KERNEL_FREEZE covers the agent kernel only).
// 2026-06-13 (release audit → fix → test → re-lock): combo per-model fallback now FALLS THROUGH on a
//   404 "no active provider for this model" (shouldStopComboFallback) instead of failing the whole
//   request — a combo listing models you don't all have a provider for now correctly reaches one you
//   do. Unit-tested + verified live (the cost_optimal "smart-cheap" combo went 404 → 200).

// flow_router Multi-Provider Dispatcher.

package router

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/creds"
	"github.com/flowork-os/flowork_Router/internal/executors"
	"github.com/flowork-os/flowork_Router/internal/providercompat"
	"github.com/flowork-os/flowork_Router/internal/safego"
	"github.com/flowork-os/flowork_Router/internal/store"
)

const httpTimeout = 300 * time.Second

var httpClient = &http.Client{Timeout: httpTimeout}

// ── OpenAI input shape (subset) ────────────────────────────────────────
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	// Tool calling (Phase 2). Passed through 1:1 for openai-compat upstreams;
	// converted to Anthropic `tools`/`tool_choice` for anthropic upstreams.
	Tools      json.RawMessage `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`

	// Additional OpenAI-spec parameters preserved through the dispatcher so
	// caller intent isn't dropped on the way to the upstream. All are
	// omitempty / json.RawMessage so a request that doesn't set them looks
	// identical on the wire.
	TopK              int             `json:"top_k,omitempty"`
	MaxCompletionTok  int             `json:"max_completion_tokens,omitempty"`
	Thinking          json.RawMessage `json:"thinking,omitempty"`
	Reasoning         json.RawMessage `json:"reasoning,omitempty"`
	EnableThinking    *bool           `json:"enable_thinking,omitempty"`
	PresencePenalty   float64         `json:"presence_penalty,omitempty"`
	FrequencyPenalty  float64         `json:"frequency_penalty,omitempty"`
	Seed              *int64          `json:"seed,omitempty"`
	Stop              json.RawMessage `json:"stop,omitempty"`
	ResponseFormat    json.RawMessage `json:"response_format,omitempty"`
	Prediction        json.RawMessage `json:"prediction,omitempty"`
	Store             *bool           `json:"store,omitempty"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
	N                 int             `json:"n,omitempty"`
	Logprobs          *bool           `json:"logprobs,omitempty"`
	TopLogprobs       *int            `json:"top_logprobs,omitempty"`
	LogitBias         json.RawMessage `json:"logit_bias,omitempty"`
	User              string          `json:"user,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Tool calling fields (Phase 2, omitempty keeps simple text path intact).
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`   // assistant → tool invocations
	ToolCallID string          `json:"tool_call_id,omitempty"` // tool result → which call
	Name       string          `json:"name,omitempty"`         // tool/function name
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// Prompt-cache breakdown (Anthropic-style). Populated when a response
	// carries cache_read_input_tokens / cache_creation_input_tokens so the
	// router can log per-request cache savings.
	PromptTokensDetails *OpenAIPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// OpenAIPromptTokensDetails mirrors the OpenAI usage breakdown for cache
// reporting. Cached = read hits; CacheCreation = writes.
type OpenAIPromptTokensDetails struct {
	CachedTokens        int `json:"cached_tokens,omitempty"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
}

// ── Anthropic shape (subset) ───────────────────────────────────────────
type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	TopP        float64            `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AnthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ── Main dispatch ──────────────────────────────────────────────────────

// DispatchChatCompletion — entry untuk POST /v1/chat/completions.
// Lookup provider berdasarkan model → forward → log → return OpenAI format.
// Resolves combo alias first kalau model match nama combo.
func DispatchChatCompletion(ctx context.Context, req OpenAIRequest) (*OpenAIResponse, int, error) {
	d, err := store.Open()
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("store open: %w", err)
	}

	settings, _ := store.LoadSettings(d)
	// No model pinned on the agent → owner doctrine: route to the HIGHEST-priority
	// ON provider's model (priority order rules when the user didn't choose). Fall
	// back to the configured DefaultModel only if no active provider exposes a
	// concrete model id.
	if req.Model == "" {
		if top := globalFallbackModels(d, nil); len(top) > 0 {
			req.Model = top[0]
		} else if settings != nil {
			req.Model = settings.DefaultModel
		}
	}
	// RTK token saver: compress large tool-result messages before forwarding.
	if settings != nil && settings.RtkTokenSaver {
		if msgs, saved := compressMessagesRTK(req.Messages); saved > 0 {
			req.Messages = msgs
			log.Printf("flow_router RTK token saver: trimmed %d chars from tool results", saved)
		}
	}

	// Caveman: append output-token-saver style instruction to the system
	// message. Pure additive mutation — translators downstream see the
	// extended system content but don't need to know about the modifier.
	if settings != nil && settings.CavemanLevel != "" {
		injectCavemanIntoRequest(&req, settings.CavemanLevel)
	}

	// Normalise tool-call ids + insert any missing tool_result follow-ups.
	// Without this, malformed payloads from upstream clients reach the
	// Anthropic API as 400s ("unmatched tool_use ids" / "invalid id pattern").
	preprocessToolCalls(&req)

	// Enrichment BERAT (constitution 20-rules + brain knowledge) cuma buat tier
	// KOMANDAN (sonnet). Crew/worker (haiku, volume ~5 call/task) di-SKIP — tugas
	// mereka fokus, doktrin+knowledge ga relevan + itu yang bakar kuota → 429.
	// (enrich_tier.go). brainInfo nil kalau di-skip → recordBrainContribution no-op.
	var brainInfo *brainEnrichInfo
	heavyEnriched := false
	if !isCrewLightModel(req.Model) {
		// Constitution: sacred-rule injection di ATAS knowledge — doktrin menang.
		maybeInjectConstitution(ctx, &req, settings)
		// Brain enrichment: knowledge + skills relevan.
		brainInfo = maybeEnrichBrain(ctx, &req, settings)
		heavyEnriched = true
	}
	// Antibody injection (mistakeenrich.go): mistakes karma-ranked sbg "antibodi"
	// anti-halu SEBELUM LLM. TETEP buat SEMUA tier (kecil + nahan halu kategori).
	maybeInjectAntibodies(ctx, &req, settings)

	// Model manager: resolve alias / custom (→ effective model + provider pin).
	resolvedModel, pinnedProvider := resolveModel(d, req.Model)
	req.Model = resolvedModel

	// Combo alias resolution: if req.Model matches a combo name, pick a model
	// from combo.Models per strategy and remember the remaining models as a
	// per-model fallback order (used when ALL providers for the picked model
	// 5xx — we then move on to the next combo model instead of giving up).
	var comboFallback []string
	if pinnedProvider == "" {
		if combo, _ := store.GetComboByName(d, req.Model); combo != nil && len(combo.Models) > 0 {
			picked := pickComboModel(combo)
			log.Printf("flow_router combo %q (%s) → model %q", combo.Name, combo.Strategy, picked)
			req.Model = picked
			comboFallback = comboFallbackOrder(combo, picked)
		}
	}

	// Per-model attempt loop: first try req.Model, then any remaining combo
	// fallbacks, then the GLOBAL priority-ordered fallback to any other ACTIVE
	// provider's model. The last part is the owner doctrine: when the requested
	// model's provider is OFF or out of quota, EVERY agent's request still lands
	// on the next ON provider by priority instead of dying with a 404. Non-combo
	// single-provider setups pay near-zero overhead (one tiny ListProviders).
	modelsToTry := append([]string{req.Model}, comboFallback...)
	nPrimary := len(modelsToTry)
	if settings == nil || settings.FallbackStrategy != "none" {
		modelsToTry = append(modelsToTry, globalFallbackModels(d, modelsToTry)...)
	}
	originalModel := modelsToTry[0]
	var lastModelErr error
	var lastModelStatus int
	for modelIdx, candidateModel := range modelsToTry {
		req.Model = candidateModel
		pin := pinnedProvider
		switch {
		case modelIdx >= nPrimary:
			// Global priority fallback candidate: re-resolve its OWN provider pin
			// (the original pin pointed at the dead provider), and run the heavy
			// enrichment we skipped when the crew-light original was chosen — so
			// the local fallback model still gets the Flowork doctrine injected.
			rm, rp := resolveModel(d, candidateModel)
			req.Model, pin = rm, rp
			if !heavyEnriched && !isCrewLightModel(req.Model) {
				maybeInjectConstitution(ctx, &req, settings)
				brainInfo = maybeEnrichBrain(ctx, &req, settings)
				heavyEnriched = true
			}
			log.Printf("flow_router priority fallback: %q unavailable → trying %q (next ON provider)", originalModel, req.Model)
		case modelIdx > 0:
			log.Printf("flow_router combo per-model fallback: trying %q", candidateModel)
		}
		resp, status, err := dispatchSingleModel(ctx, d, req, settings, brainInfo, pin)
		if err == nil && resp != nil {
			return resp, status, nil
		}
		lastModelErr = err
		lastModelStatus = status
		// Decide whether to try the next candidate. A combo exists precisely to list
		// alternatives, so "no active provider serves THIS model" (404) must fall through to the next
		// listed model — not fail the whole request. Upstream 5xx also falls through. Only a
		// request-/policy-level 4xx (400 malformed, 401 bad inbound auth, 403 disabled/not-permitted)
		// is identical across models, so we stop early and surface it.
		stop := shouldStopComboFallback(status)
		// Owner doctrine override: if THIS failure is an availability failure
		// (provider off / quota / 5xx) and the NEXT candidate is a global ON
		// provider, never give up — let the priority fallback serve it.
		if stop && modelIdx+1 < len(modelsToTry) && modelIdx+1 >= nPrimary && availabilityFailure(status) {
			stop = false
		}
		if stop {
			break
		}
	}
	return nil, lastModelStatus, lastModelErr
}

// ═══════════════════════════════════════════════════════════════════════════
// STABLE — DO NOT MODIFY without owner approval.  (locked 2026-06-15)
//
// TUJUAN (biar AI berikutnya paham, jangan dibongkar):
// Flowork = "rumah AI" dengan 2 brain TERPISAH by design:
//   • ROUTER brain  — atur provider + suntik doktrin/konstitusi (persona via RAG).
//   • AGENT brain   — tiap agent punya model sendiri (kv router_model → FLOWORK_AGENT_CONFIG).
// Owner doctrine (2026-06-15): "kalau provider mati / token abis → OTOMATIS pindah
// ke model yang ON sesuai urutan PRIORITY, berlaku SEMUA agent." Diversity penting:
// 1 rumah banyak karakter (agent A=Claude, B=lokal) biar nggak mikir sama → JANGAN
// paksa semua ke 1 model (jangan ubah jadi priority-first; model pilihan agent menang
// dulu, baru failover). Failover ini yang bikin sistem selamat pas langganan cloud abis.
//
// availabilityFailure + globalFallbackModels + loop fallback di DispatchChatCompletion
// = mekanisme failover-by-priority. Mirror di dispatcher_stream.go. shouldStopComboFallback
// (combo_fallback_test.go) LOCKED terpisah — jangan disentuh.
//
// CATATAN OPERASIONAL (mahal dipelajari): provider lokal (llama-server) WAJIB punya
// context cukup (-c ≥ 32768). Doktrin+brain bikin prompt membengkak ~9-12k token; kalau
// -c 8192, llama-server tolak 400 "exceed_context_size" → tiap request lokal failover
// ke cloud (keliatan "agent nyangkut di Claude" padahal config-nya bener). Lihat memory
// [[flowork-router-failover]].
// ═══════════════════════════════════════════════════════════════════════════

// availabilityFailure reports whether an upstream status means "this provider
// can't serve right now" (so the request should slide to the next ON provider
// by priority) rather than a permanent request error every provider rejects
// identically. Covers: 404 no active provider, 408 timeout, 429 rate/quota
// ("token abis"), 402 out-of-credit, and all 5xx. (owner doctrine: auto-failover)
func availabilityFailure(status int) bool {
	switch status {
	case http.StatusNotFound, http.StatusRequestTimeout,
		http.StatusTooManyRequests, http.StatusPaymentRequired:
		return true
	}
	return status >= 500
}

// globalFallbackModels returns one concrete model per ACTIVE provider, in
// provider-priority order (ListProviders is ORDER BY priority ASC), excluding
// models already queued in `tried` and any wildcard ("*"/"claude-*") entries
// (the upstream needs a concrete model id). This is the priority-ordered safety
// net behind every request. (owner doctrine)
func globalFallbackModels(d *sql.DB, tried []string) []string {
	provs, err := store.ListProviders(d)
	if err != nil {
		return nil
	}
	skip := make(map[string]bool, len(tried))
	for _, m := range tried {
		skip[strings.ToLower(strings.TrimSpace(m))] = true
	}
	var out []string
	for _, p := range provs {
		if !p.IsActive {
			continue
		}
		models, _ := p.Data[store.CfgModels].([]any)
		for _, mm := range models {
			ms, ok := mm.(string)
			if !ok {
				continue
			}
			ms = strings.TrimSpace(ms)
			if ms == "" || ms == "*" || strings.HasSuffix(ms, "*") {
				continue // need a concrete model id the upstream accepts
			}
			key := strings.ToLower(ms)
			if skip[key] {
				continue
			}
			skip[key] = true
			out = append(out, ms)
			break // one concrete model per provider is enough
		}
	}
	return out
}

// dispatchSingleModel runs the full provider-selection + try-each-candidate
// loop for ONE concrete model. Extracted so DispatchChatCompletion can walk
// combo fallbacks on 5xx.
func dispatchSingleModel(ctx context.Context, d *sql.DB, req OpenAIRequest, settings *store.Settings, brainInfo *brainEnrichInfo, pinnedProvider string) (*OpenAIResponse, int, error) {
	matches, err := store.FindActiveByModel(d, req.Model)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("find provider: %w", err)
	}
	if pinnedProvider != "" {
		matches = pinProvider(d, matches, pinnedProvider)
	}
	if len(matches) == 0 {
		return nil, http.StatusNotFound, fmt.Errorf("no active provider supports model %q", req.Model)
	}
	// Drop providers where this model is disabled.
	if matches = filterDisabled(d, matches, req.Model); len(matches) == 0 {
		return nil, http.StatusForbidden, fmt.Errorf("model %q is disabled", req.Model)
	}

	// Inbound API-key scope: drop providers the key is not allowed to use.
	keyID := apiKeyID(ctx)
	if key := APIKeyFromContext(ctx); key != nil {
		matches = filterByAllowedProviders(matches, key)
		if len(matches) == 0 {
			return nil, http.StatusForbidden, fmt.Errorf("api key %q not permitted for any provider serving model %q", key.Name, req.Model)
		}
	}

	// Per-intent multiplexing: a private prompt may only go to a local-tagged
	// provider — refuse rather than leak to cloud.
	if settings != nil && settings.IntentRouting.Enabled && promptIsPrivate(req, settings.IntentRouting.PrivatePatterns) {
		tag := settings.IntentRouting.PrivateTag
		if tag == "" {
			tag = "local"
		}
		local := filterByTag(matches, tag)
		if len(local) == 0 {
			return nil, http.StatusForbidden, fmt.Errorf("private prompt: no provider tagged %q available — refusing to route to cloud", tag)
		}
		matches = local
		log.Printf("flow_router intent-routing: private prompt → %d provider(s) tagged %q", len(local), tag)
	}

	// Cost-tier routing: classify request → filter providers by tier:* tag.
	// Skips when user explicitly named a model an active provider serves.
	if settings != nil && settings.CostRouting.Enabled {
		if !(settings.CostRouting.HonorExplicitModel && hasActiveProviderForModel(matches, req.Model)) {
			tier := ClassifyCost(req, settings.CostRouting)
			if tiered := filterByTier(matches, tier); len(tiered) > 0 {
				matches = tiered
				log.Printf("flow_router cost-routing: tier=%s → %d provider(s)", tier, len(tiered))
			}
		}
	}

	// Fallback strategy: reorder candidates (priority_ordered = unchanged).
	if settings != nil {
		matches = applyFallbackStrategy(matches, settings.FallbackStrategy, req.Model)
	}

	// Per-(provider,model) cooldown: push currently locked pairs to the back so a
	// healthy provider is tried first. Never drops them — a fully-locked model
	// still gets a last-resort attempt (zero regression vs the pre-lock loop).
	matches = reorderByModelLock(matches, req.Model)

	// Try candidates in order, fallback to next on error
	var lastErr error
	startTotal := time.Now()
	for _, p := range matches {
		// 429-aware retry (ratelimit.go): rate-limit = provider SEHAT tapi kuota
		// mepet → TUNGGU (backoff) + ulang provider SAMA, jangan langsung gagal.
		var resp *OpenAIResponse
		var status int
		var err error
		for attempt := 0; ; attempt++ {
			start := time.Now()
			resp, status, err = forwardToProvider(ctx, &p, req)
			latencyMs := time.Since(start).Milliseconds()
			// Log usageHistory (best-effort). Capture per-attempt biar ga ke-race.
			lr, ls, le := resp, status, err
			safego.GoLabel("logUsage", func() {
				logUsage(d, keyID, p.ID, req.Model, lr, ls, le, latencyMs)
			})
			if status != http.StatusTooManyRequests || attempt >= maxRateLimitRetries {
				break
			}
			wait := backoffDuration(attempt)
			log.Printf("flow_router 429 (rate-limit) model=%s provider=%s → antri %v lalu retry (%d/%d)",
				req.Model, p.Name, wait, attempt+1, maxRateLimitRetries)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, http.StatusGatewayTimeout, ctx.Err()
			}
		}

		if err == nil && resp != nil {
			clearModelLock(p.ID, req.Model) // recovered → prefer this pair again
			log.Printf("flow_router dispatch model=%s → provider=%s tokens=%d",
				req.Model, p.Name, resp.Usage.TotalTokens)
			recordBrainContribution(d, settings, brainInfo, answerText(resp))
			// Feedback loop (mistakefeedback.go): kalau response masih halu
			// kategori → naikin karma antibody (self-learning). Async, best-effort.
			respCopy := resp
			settingsCopy := settings
			safego.GoLabel("antibodyFeedback", func() {
				maybeReinforceAntibody(context.Background(), respCopy, settingsCopy)
			})
			return resp, status, nil
		}
		lastErr = err
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		// 429 (retry habis): JANGAN lock — provider sehat, cuma rate-limited. Lock
		// cuma bikin request berikutnya ikut gagal (cascade). Selain itu baru lock.
		if status != http.StatusTooManyRequests {
			lockModel(p.ID, req.Model, status, errText)
		}
		log.Printf("flow_router fallback model=%s provider=%s failed (%v), trying next", req.Model, p.Name, err)
	}

	log.Printf("flow_router ALL providers exhausted model=%s total=%dms", req.Model, time.Since(startTotal).Milliseconds())
	return nil, http.StatusBadGateway, fmt.Errorf("all providers failed; last error: %w", lastErr)
}

// forwardToProvider — dispatch ke provider tertentu dengan format proper.
func forwardToProvider(ctx context.Context, p *store.ProviderConnection, req OpenAIRequest) (*OpenAIResponse, int, error) {
	// Bergantian (ratelimit.go): max N request ke provider barengan, sisanya antri.
	// Slot cuma dipegang selama HTTP call; backoff-sleep (di loop) di luar slot.
	acquireDispatchSlot()
	defer releaseDispatchSlot()

	format, _ := p.Data[store.CfgFormat].(string)
	baseURL, _ := p.Data[store.CfgBaseURL].(string)

	// Auto-resolve format + baseURL when the provider record uses one of the
	// "openai-compatible-…" / "anthropic-compatible-…" name prefixes and the
	// operator didn't supply the explicit fields. Explicit values always win.
	if format == "" {
		if resolved := providercompat.ResolveFormat(p.Provider); resolved != "" {
			format = resolved
		}
	}
	if baseURL == "" {
		baseURL = providercompat.ResolveBaseURL(p.Provider, baseURL)
	}

	if baseURL == "" {
		return nil, 0, fmt.Errorf("provider %s missing baseUrl", p.ID)
	}

	// Power saver (idle-sleep): wake the local engine if it was unloaded, and hold it
	// loaded until this request returns. No-op for cloud. See router/llm_idle_sleep.go.
	defer wakeLocalIfNeeded(baseURL)()

	// Vendor executor (non-stream path).
	if ex := executors.Get(format); ex != nil {
		body, u, st, err := ex.NonStream(ctx, p, executorRequest(req))
		if err != nil {
			return nil, st, err
		}
		var resp OpenAIResponse
		if jerr := json.Unmarshal(body, &resp); jerr != nil {
			return nil, http.StatusBadGateway, fmt.Errorf("executor %s decode: %w", format, jerr)
		}
		if resp.Usage.TotalTokens == 0 {
			resp.Usage.PromptTokens = u.PromptTokens
			resp.Usage.CompletionTokens = u.CompletionTokens
			resp.Usage.TotalTokens = u.TotalTokens
		}
		return &resp, st, nil
	}

	switch format {
	case "anthropic":
		return forwardAnthropic(ctx, p, baseURL, req)
	case "openai", "":
		return forwardOpenAICompat(ctx, p, baseURL, req)
	case "gemini":
		return forwardGemini(ctx, p, baseURL, req)
	default:
		return nil, 0, fmt.Errorf("unknown format: %s", format)
	}
}

// forwardOpenAICompat — passthrough untuk provider yang udah OpenAI-compat
// (local llama-server, OpenAI API, DeepSeek, Groq, Together AI, etc).
func forwardOpenAICompat(ctx context.Context, p *store.ProviderConnection, baseURL string, req OpenAIRequest) (*OpenAIResponse, int, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal: %w", err)
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := applyAuth(httpReq, p); err != nil {
		return nil, http.StatusUnauthorized, err
	}

	resp, err := outboundClient(ctx).Do(httpReq)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var out OpenAIResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("parse resp: %w", err)
	}
	return &out, http.StatusOK, nil
}

// forwardAnthropic — translate OpenAI → Anthropic Messages, forward, translate back.
// When the request carries tools/tool-role messages, the rich tool path runs
// (buildAnthropicToolBody + parseAnthropicToolResponse). Otherwise the proven
// simple text path is used unchanged.
func forwardAnthropic(ctx context.Context, p *store.ProviderConnection, baseURL string, req OpenAIRequest) (*OpenAIResponse, int, error) {
	if hasToolContext(req) {
		return forwardAnthropicWithTools(ctx, p, baseURL, req)
	}
	// Translate request
	anthrReq := AnthropicRequest{
		Model:       normalizeClaudeModel(req.Model),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if anthrReq.MaxTokens <= 0 {
		anthrReq.MaxTokens = 4096
	}
	var sysParts []string
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			sysParts = append(sysParts, m.Content)
		case "user", "assistant":
			anthrReq.Messages = append(anthrReq.Messages, AnthropicMessage{Role: m.Role, Content: m.Content})
		}
	}
	if len(sysParts) > 0 {
		anthrReq.System = strings.Join(sysParts, "\n\n")
	}

	body, err := json.Marshal(anthrReq)
	if err != nil {
		return nil, 0, fmt.Errorf("marshal anthropic: %w", err)
	}

	// Claude OAuth identity cloaking (anti-ban): billing header + fake user_id.
	// No tools on this path, so only the identity cloak applies. No-op for
	// non-OAuth providers.
	if claudeUsesOAuth(p) {
		body = applyClaudeIdentityCloak(body, "")
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("build req: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("User-Agent", "claude-cli/1.0.0 (flow_router)")
	if err := applyAuth(httpReq, p); err != nil {
		return nil, http.StatusUnauthorized, err
	}

	resp, err := outboundClient(ctx).Do(httpReq)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("upstream: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("anthropic %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var anthrResp AnthropicResponse
	if err := json.Unmarshal(respBody, &anthrResp); err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("parse anthropic: %w", err)
	}

	// Translate response
	var content string
	for _, c := range anthrResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}
	stopReason := "stop"
	switch anthrResp.StopReason {
	case "end_turn", "stop_sequence":
		stopReason = "stop"
	case "max_tokens":
		stopReason = "length"
	case "tool_use":
		stopReason = "tool_calls"
	}
	return &OpenAIResponse{
		ID:      anthrResp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []OpenAIChoice{{
			Index:        0,
			Message:      OpenAIMessage{Role: "assistant", Content: content},
			FinishReason: stopReason,
		}},
		Usage: OpenAIUsage{
			PromptTokens:     anthrResp.Usage.InputTokens,
			CompletionTokens: anthrResp.Usage.OutputTokens,
			TotalTokens:      anthrResp.Usage.InputTokens + anthrResp.Usage.OutputTokens,
		},
	}, http.StatusOK, nil
}

// applyAuth attach proper auth header based on provider authType.
func applyAuth(req *http.Request, p *store.ProviderConnection) error {
	switch p.AuthType {
	case store.AuthTypeNone:
		return nil // local llama, no auth
	case store.AuthTypeAPIKey:
		k, _ := p.Data[store.CfgAPIKey].(string)
		if k == "" {
			return fmt.Errorf("provider %s missing apiKey", p.ID)
		}
		// Anthropic uses x-api-key, OpenAI uses Authorization Bearer
		if p.Provider == "anthropic" {
			req.Header.Set("x-api-key", k)
		} else {
			req.Header.Set("Authorization", "Bearer "+k)
		}
		return nil
	case store.AuthTypeSubscription:
		// Read live from credential source
		src, _ := p.Data[store.CfgTokenSource].(string)
		switch src {
		case "claude_credentials":
			// LoadValid auto-refreshes an expired subscription token (OAuth refresh_token grant) and
			// persists the rotated token, so Claude keeps working unattended on a device with NO Claude
			// Code to refresh the file for us (Android / USB appliance). On a normal desktop the token
			// is already fresh (Claude Code maintains it) so this is a plain read. When no refresh is
			// possible it returns a clear "re-import via OAuth Imports → Browse" error.
			c, err := creds.LoadValid()
			if err != nil {
				return fmt.Errorf("claude creds: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+c.ClaudeAiOauth.AccessToken)
			return nil
		case "codex_auth":
			tok, err := creds.LoadCodexToken()
			if err != nil {
				return fmt.Errorf("codex auth: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+tok)
			return nil
		case "cursor_session":
			tok, err := creds.LoadCursorToken()
			if err != nil {
				return fmt.Errorf("cursor auth: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+tok)
			return nil
		default:
			return fmt.Errorf("unknown subscription tokenSource: %s", src)
		}
	}
	return fmt.Errorf("unknown authType: %s", p.AuthType)
}

// pickComboModel — strategy-aware model selection dari combo.
// Priority: return first. RoundRobin: cycle via index counter. Random: random.
// CostOptimal: pick model dengan known lowest pricing.
// comboFallbackOrder returns the combo's models in the order to retry after
// `picked` has been tried. The picked model is excluded; remaining models
// keep their original list order (priority semantics). Returns nil when the
// combo has fewer than 2 models so the caller skips the fallback loop.
// shouldStopComboFallback reports whether a failed combo-model attempt should STOP the per-model
// fallback loop instead of trying the next listed model. A combo lists alternatives, so a per-model
// failure — 404 "no active provider for this model" or any 5xx upstream — falls through to the next
// model. A request-/policy-level 4xx (400 malformed body, 401 bad inbound auth, 403 disabled or
// key-not-permitted) is identical across models, so it stops early and surfaces the real cause.
func shouldStopComboFallback(status int) bool {
	return status > 0 && status < 500 && status != http.StatusNotFound
}

func comboFallbackOrder(c *store.Combo, picked string) []string {
	if c == nil || len(c.Models) < 2 {
		return nil
	}
	out := make([]string, 0, len(c.Models)-1)
	for _, m := range c.Models {
		if m == picked {
			continue
		}
		out = append(out, m)
	}
	return out
}

func pickComboModel(c *store.Combo) string {
	if len(c.Models) == 0 {
		return ""
	}
	switch c.Strategy {
	case store.ComboStrategyRoundRobin:
		i := nextRoundRobin("combo:"+c.ID, len(c.Models))
		return c.Models[i]
	case store.ComboStrategyRandom:
		// Use time.Now nanos modulo as cheap PRNG (no crypto needed for routing).
		// uint64 conversion avoids a negative index: on 32-bit int builds (or any
		// nanos value that overflows int) `int(x) % len` can be negative, which
		// would panic with index-out-of-range and crash the request goroutine.
		return c.Models[int(uint64(time.Now().UnixNano())%uint64(len(c.Models)))]
	case store.ComboStrategyCostOptimal:
		// Pick model yang harga input+output terendah (estimateCost as proxy).
		bestModel := c.Models[0]
		bestCost := estimateCost(bestModel, 1000, 1000) // 1k+1k token sample
		for _, m := range c.Models[1:] {
			cost := estimateCost(m, 1000, 1000)
			if cost > 0 && (bestCost == 0 || cost < bestCost) {
				bestModel = m
				bestCost = cost
			}
		}
		return bestModel
	default: // priority
		return c.Models[0]
	}
}

func normalizeClaudeModel(m string) string {
	m = strings.TrimSpace(m)
	for _, prefix := range []string{"cc/", "anthropic/", "claude/"} {
		m = strings.TrimPrefix(m, prefix)
	}
	if m == "" {
		return "claude-haiku-4-5"
	}
	return m
}

// logUsage — append-only request log + daily aggregate.
// Called async (goroutine) per dispatch — never blocks caller.
func logUsage(d any, apiKeyID, providerID, model string, resp *OpenAIResponse, status int, errIn error, latencyMs int64) {
	db, ok := d.(*sql.DB)
	if !ok || db == nil {
		return
	}
	entry := &store.LogEntry{
		APIKeyID:   apiKeyID,
		ProviderID: providerID,
		Model:      model,
		StatusCode: status,
		LatencyMs:  latencyMs,
	}
	if errIn != nil {
		entry.Error = errIn.Error()
	}
	if resp != nil {
		entry.PromptTokens = resp.Usage.PromptTokens
		entry.CompletionTokens = resp.Usage.CompletionTokens
		entry.TotalTokens = resp.Usage.TotalTokens
		entry.CostUsd = estimateCost(model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
	_ = store.LogRequest(db, entry)
}

// estimateCost — rough USD cost estimate per million tokens.
// estimateCost — DATA-driven cost estimate. Reads the pricing table (which
// the user can edit via /api/pricing and which SeedDefaultPricing seeds),
// so there is NO hardcoded rate map. Unknown model → 0 (local/free).
func estimateCost(model string, promptTok, complTok int) float64 {
	d, err := store.Open()
	if err != nil {
		return 0
	}
	pr, err := store.LookupPricingByModel(d, model)
	if err != nil || pr == nil {
		return 0 // unknown model = treat as free (e.g. local llama)
	}
	return (float64(promptTok)/1e6)*pr.InputUsdPer1M + (float64(complTok)/1e6)*pr.OutputUsdPer1M
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
