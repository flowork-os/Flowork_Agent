// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30
// Reason: WASM host function surface. CRITICAL security boundary. Audit pass:
//   - Cap gate per host fn (state:write, net:fetch:, exec:, rpc:call:,
//     time:read post-fix)
//   - pluginID dari ctx via unexported key (anti spoof)
//   - Memory read/write bounds checked (m.Memory().Read/Write returns bool)
//   - Body cap: netFetch 4MB default/8MB max, slash 8KB, err msg 400 char
//   - Timeout: netFetch 60s/5m, exec 30s/5m, http client 120s
//   - Cross-OS shell: /bin/sh vs cmd.exe via resolveBinary
//   - Fix 2026-05-30: host_time_now_ms now gates time:read cap (sebelumnya
//     silent allow regardless of manifest declaration).
//   - Note: empty pluginID skip cap check di netFetch/execRun/rpcCall —
//     mitigated by kernel always setting via WithGuestPluginID in Instance.
//     Defensive future improvement: reject empty pluginID outright.
//
// Host module — kernel-side implementation of `flowork.*` host imports
// yang plugin pakai untuk akses capability.
//
// Phase 5 scope:
//
//   host_exec_run(reqPtr, reqLen, outPtr, outMax) → bytesWritten
//
// Capability gate dieksekusi oleh broker; kalau ditolak, host nulis JSON
// `{"error":"..."}` ke buffer plugin dan return panjangnya. Plugin
// surface error itu ke caller-nya.

package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// CapsChecker — kernel pasang fungsi ini dari broker supaya host module
// tidak langsung depend ke broker (mencegah cycle runtime ↔ broker).
type CapsChecker func(pluginID, capability string) bool

// InstanceResolver — runtime callback supaya host_rpc_call bisa cari
// Instance target plugin. Phase 7-8 inject via Bootstrap.
type InstanceResolver func(pluginID string) *Instance

// InteractionLogger — kernel-side callback yang route interaction log dari
// guest plugin (mr-flow Telegram, dst.) ke `agentdb.Store.LogInteraction`
// punya plugin itu sendiri. Plugin cuma log ke state.db nya sendiri —
// bukan attack surface (lihat roadmap section 1, standar section 11).
type InteractionLogger func(pluginID, channel, direction, actor, content string, metadata map[string]any) error

// DecisionLogger — kernel-side callback yang route decision log dari guest
// plugin ke `agentdb.Store.LogDecision`. Pola sama InteractionLogger —
// per-warga isolation, pluginID dari ctx (anti spoof). Return ID supaya
// caller bisa reuse buat cross-ref future. Lihat roadmap section 3.
type DecisionLogger func(pluginID, decisionType, rationale, outcome string, inputs map[string]any, refInteractionID int64) (int64, error)

// KarmaUpdater — kernel-side callback yang route karma update dari guest
// plugin ke `agentdb.Store.IncrementKarma` atau `AverageUpdateKarma`.
// op = 'increment' atau 'average'. Plugin cuma update karma diri sendiri
// (pluginID dari ctx). Lihat roadmap section 5.
type KarmaUpdater func(pluginID, op, key string, value float64) (float64, error)

// SlashDispatcher — kernel-side callback yang dispatch slash command via
// slashcmd.Dispatch + log invocation ke agent state.db. Plugin (mr-flow)
// terima full text mis. "/help", host parse + run + return result.Text.
// Lihat roadmap section 17 (Mr.Flow Telegram integration).
type SlashDispatcher func(pluginID, text, caller string) (result string, cmdName string, err error)

// hostState dibawa per-instantiation supaya host_exec_run tau pluginID
// pemanggil. Setiap instance plugin diberi ApiContext dengan pluginID
// melalui WithGuestPluginID() saat Call.
type hostState struct {
	caps        CapsChecker
	resolver    InstanceResolver
	interaction InteractionLogger
	decision    DecisionLogger
	karma       KarmaUpdater
	slash       SlashDispatcher
	http        *http.Client
}

// pluginIDKey — context key untuk pass pluginID ke host functions.
type pluginIDKey struct{}

// WithGuestPluginID mengembalikan ctx yang membawa pluginID. Setiap RPC
// call ke plugin harus dimulai dengan ctx ini supaya host func tau siapa
// yang manggil.
func WithGuestPluginID(ctx context.Context, pluginID string) context.Context {
	return context.WithValue(ctx, pluginIDKey{}, pluginID)
}

func guestPluginID(ctx context.Context) string {
	v, _ := ctx.Value(pluginIDKey{}).(string)
	return v
}

type execReq struct {
	Binary    string   `json:"binary"`
	Args      []string `json:"args"`
	Cwd       string   `json:"cwd"`
	TimeoutMS int      `json:"timeout_ms"`
}

type execResp struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// registerFloworkHost build dan instantiate host module "flowork".
// Phase 8: nambah `host_rpc_call` setelah `host_exec_run` + `host_net_fetch`.
// Roadmap section 1: nambah `host_log_interaction` untuk episodic log.
// Roadmap section 3: nambah `host_log_decision` untuk decisions audit trail.
// Roadmap section 5: nambah `host_karma_update` untuk metric self.
func (r *Runtime) registerFloworkHost(ctx context.Context, caps CapsChecker, resolver InstanceResolver, interaction InteractionLogger, decision DecisionLogger, karma KarmaUpdater, slash SlashDispatcher) error {
	st := &hostState{
		caps:        caps,
		resolver:    resolver,
		interaction: interaction,
		decision:    decision,
		karma:       karma,
		slash:       slash,
		http: &http.Client{
			// 300s (was 120s, owner-approved 2026-06-16): this is only a CEILING — each
			// netFetch already sets a per-request context deadline (default 60s, max 5min,
			// see netFetch timeout logic). 120s here silently CAPPED a valid longer per-
			// request timeout_ms (e.g. a channel's bus.request to a slow LOCAL-model crew
			// orchestrator) → "loket: no response" on Telegram. 300s = the per-request max,
			// so a normal 60s fetch is unaffected; only an explicit long timeout is honored.
			Timeout: 300 * time.Second,
			// SSRF guard on the WASM net:fetch host-func: block cloud-metadata /
			// link-local at dial time (initial + any redirect). Loopback stays
			// allowed — agents MUST reach the self-API (:1987) and router (:2402).
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           hostFetchDial,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}

	_, err := r.rt.NewHostModuleBuilder("flowork").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.execRun(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_exec_run").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.netFetch(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_net_fetch").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.rpcCall(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_rpc_call").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.logInteraction(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_log_interaction").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.logDecision(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_log_decision").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.karmaUpdate(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_karma_update").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context) uint64 {
			// Wall-clock ms since epoch. Workaround TinyGo wasi time.Since()
			// precision bug — Mr.Flow pakai untuk avg_response_ms accurate.
			// Capability gate: time:read. Plugin tanpa cap → 0 (silent
			// denial — anti exception flood. Tetap log denial via st.caps
			// callback kalau future audit nya pengen di-trace).
			if st.caps != nil {
				pid := guestPluginID(ctx)
				if pid != "" && !st.caps(pid, "time:read") {
					return 0
				}
			}
			return uint64(time.Now().UnixMilli())
		}).
		Export("host_time_now_ms").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
			return st.slashDispatch(ctx, m, reqPtr, reqLen, outPtr, outMax)
		}).
		Export("host_slash_dispatch").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("instantiate flowork host: %w", err)
	}
	return nil
}

type logDecisionReq struct {
	DecisionType     string         `json:"decision_type"`
	Rationale        string         `json:"rationale"`
	Outcome          string         `json:"outcome,omitempty"`
	Inputs           map[string]any `json:"inputs,omitempty"`
	RefInteractionID int64          `json:"ref_interaction_id,omitempty"`
}

type logDecisionResp struct {
	OK    bool   `json:"ok"`
	ID    int64  `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
}

// logDecision — host import `host_log_decision`. Plugin kirim
// {decision_type, rationale, outcome?, inputs?, ref_interaction_id?},
// host route ke per-agent state.db via decision callback. Plugin cuma log
// ke DB nya sendiri (pluginID dari ctx, anti spoof).
//
// Capability gate: `state:write` (sama dengan host_log_interaction).
// JANGAN inject decisions ke system prompt (anti over-prompt, lihat
// standar section 11).
func (st *hostState) logDecision(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)
	if pluginID == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("pluginID missing in context"))
	}

	if st.caps != nil {
		const want = "state:write"
		if !st.caps(pluginID, want) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+want))
		}
	}

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req logDecisionReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}

	if st.decision == nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decision logger not wired"))
	}
	id, err := st.decision(pluginID, req.DecisionType, req.Rationale, req.Outcome, req.Inputs, req.RefInteractionID)
	if err != nil {
		msg := err.Error()
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		out, _ := json.Marshal(logDecisionResp{OK: false, Error: msg})
		return writeJSONOrCrop(m, outPtr, outMax, out)
	}
	out, _ := json.Marshal(logDecisionResp{OK: true, ID: id})
	return writeJSONOrCrop(m, outPtr, outMax, out)
}

type karmaUpdateReq struct {
	Op    string  `json:"op"`    // 'increment' | 'average'
	Key   string  `json:"key"`   // 'success_count' | 'fail_count' | 'avg_response_ms' | dst
	Value float64 `json:"value"` // delta untuk increment, sample untuk average
}

type karmaUpdateResp struct {
	OK      bool    `json:"ok"`
	Current float64 `json:"current,omitempty"` // value setelah update
	Error   string  `json:"error,omitempty"`
}

// karmaUpdate — host import `host_karma_update`. Plugin kirim {op, key, value},
// host route ke per-agent state.db via karma callback. Plugin cuma update
// karma diri sendiri (pluginID dari ctx, anti spoof).
//
// Capability gate: `state:write` (sama dengan host_log_interaction +
// host_log_decision). Section 5 roadmap.
type slashDispatchReq struct {
	Text   string `json:"text"`
	Caller string `json:"caller,omitempty"`
}

type slashDispatchResp struct {
	OK      bool   `json:"ok"`
	Command string `json:"command,omitempty"`
	Text    string `json:"text,omitempty"`
	Error   string `json:"error,omitempty"`
}

// slashDispatch — host import `host_slash_dispatch`. Plugin kirim
// {text, caller?} (e.g. "/help"), host parse + dispatch via slashcmd
// registry + log invocation, balas {ok, command, text, error}.
//
// Capability gate: `state:write` (sama dengan host_log_decision —
// kalau guest boleh log invocation, boleh dispatch).
//
// Plugin cuma dispatch & log untuk dirinya sendiri (pluginID dari ctx).
// Roadmap section 17.
func (st *hostState) slashDispatch(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)
	if pluginID == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("pluginID missing in context"))
	}
	if st.caps != nil {
		const want = "state:write"
		if !st.caps(pluginID, want) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+want))
		}
	}

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req slashDispatchReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}

	if st.slash == nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("slash dispatcher not wired"))
	}
	result, cmdName, err := st.slash(pluginID, req.Text, req.Caller)
	if err != nil {
		msg := err.Error()
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		out, _ := json.Marshal(slashDispatchResp{OK: false, Command: cmdName, Error: msg})
		return writeJSONOrCrop(m, outPtr, outMax, out)
	}
	// Cap result text 8KB supaya guest buffer ngga overflow.
	if len(result) > 8192 {
		result = result[:8192] + "…[truncated]"
	}
	out, _ := json.Marshal(slashDispatchResp{OK: true, Command: cmdName, Text: result})
	return writeJSONOrCrop(m, outPtr, outMax, out)
}

func (st *hostState) karmaUpdate(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)
	if pluginID == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("pluginID missing in context"))
	}

	if st.caps != nil {
		const want = "state:write"
		if !st.caps(pluginID, want) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+want))
		}
	}

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req karmaUpdateReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}

	if st.karma == nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("karma updater not wired"))
	}
	current, err := st.karma(pluginID, req.Op, req.Key, req.Value)
	if err != nil {
		msg := err.Error()
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		out, _ := json.Marshal(karmaUpdateResp{OK: false, Error: msg})
		return writeJSONOrCrop(m, outPtr, outMax, out)
	}
	out, _ := json.Marshal(karmaUpdateResp{OK: true, Current: current})
	return writeJSONOrCrop(m, outPtr, outMax, out)
}

type logInteractionReq struct {
	Channel   string         `json:"channel"`
	Direction string         `json:"direction"`
	Actor     string         `json:"actor"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type logInteractionResp struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// logInteraction — host import `host_log_interaction`. Plugin kirim
// {channel, direction, actor, content, metadata}, host route ke per-agent
// state.db via interaction callback. Plugin cuma log ke DB nya sendiri
// (pluginID di-resolve dari ctx) — ngga bisa write ke agent lain.
//
// JANGAN inject hasil log ke system prompt (anti over-prompt, lihat
// standar_ai_agent.md section 11).
func (st *hostState) logInteraction(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)
	if pluginID == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("pluginID missing in context"))
	}

	// Capability gate: plugin wajib declare `state:write` di manifest. Walau
	// plugin cuma log ke DB-nya sendiri (pluginID dari ctx anti-spoof), gate
	// ini tetap explicit supaya audit trail jelas + plugin tanpa cap ngga
	// sembarang spam tabel interactions.
	if st.caps != nil {
		const want = "state:write"
		if !st.caps(pluginID, want) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+want))
		}
	}

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req logInteractionReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}

	if st.interaction == nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("interaction logger not wired"))
	}
	if err := st.interaction(pluginID, req.Channel, req.Direction, req.Actor, req.Content, req.Metadata); err != nil {
		// Cap error message supaya muat di guest buffer (lihat mr-flow logBuf).
		msg := err.Error()
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		out, _ := json.Marshal(logInteractionResp{OK: false, Error: msg})
		return writeJSONOrCrop(m, outPtr, outMax, out)
	}
	out, _ := json.Marshal(logInteractionResp{OK: true})
	return writeJSONOrCrop(m, outPtr, outMax, out)
}

type rpcReq struct {
	Target string          `json:"target"` // "<plugin-id>.<method>"
	Args   json.RawMessage `json:"args,omitempty"`
}

// rpcCall — host import `host_rpc_call`. Plugin kirim `{target, args}`,
// kernel resolve target plugin Instance, invoke Call, forward response.
// Capability gate: `rpc:call:<plugin-id>` atau `rpc:call:<plugin-id>.<method>`.
func (st *hostState) rpcCall(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req rpcReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}
	req.Target = strings.TrimSpace(req.Target)
	if req.Target == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("target required (<plugin-id>.<method>)"))
	}
	dot := strings.IndexByte(req.Target, '.')
	if dot <= 0 || dot == len(req.Target)-1 {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("target format: <plugin-id>.<method>"))
	}
	targetPlugin := req.Target[:dot]
	targetMethod := req.Target[dot+1:]

	// Capability gate. Approval bisa berbentuk:
	//   rpc:call:<plugin-id>           (semua method)
	//   rpc:call:<plugin-id>.<method>  (method spesifik)
	if st.caps != nil && pluginID != "" {
		full := "rpc:call:" + req.Target
		broad := "rpc:call:" + targetPlugin
		if !st.caps(pluginID, full) && !st.caps(pluginID, broad) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+full))
		}
	}

	if st.resolver == nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("kernel resolver not wired"))
	}
	inst := st.resolver(targetPlugin)
	if inst == nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("plugin not loaded: "+targetPlugin))
	}

	argsBytes := []byte("{}")
	if len(req.Args) > 0 {
		argsBytes = req.Args
	}
	resp, err := inst.Call(ctx, targetMethod, argsBytes)
	if err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("rpc call: "+err.Error()))
	}
	if len(resp) == 0 {
		return writeJSONOrCrop(m, outPtr, outMax, []byte("{}"))
	}
	// Forward apa adanya — caller plugin parse JSON sendiri.
	return writeJSONOrCrop(m, outPtr, outMax, resp)
}

type netFetchReq struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyB64     string            `json:"body_base64,omitempty"`
	TimeoutMS   int               `json:"timeout_ms,omitempty"`
	MaxRespByte int               `json:"max_resp_bytes,omitempty"`
}

type netFetchResp struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	BodyB64 string            `json:"body_base64,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// netFetch — implementasi `host_net_fetch`. Plugin kirim request JSON,
// host check capability `net:fetch:<url>` (atau pattern), eksekusi HTTP,
// kembali respons (status + headers + body base64) di buffer plugin.
// hostFetchDial blocks dials to cloud-metadata / link-local addresses (the
// classic SSRF pivot to steal cloud IAM creds) while allowing loopback (the
// self-API + router that agents legitimately need). Runs on every dial, so it
// also catches redirects. Public + private-LAN destinations are allowed (a
// self-hosted agent may reach the owner's own services); only the always-hostile
// link-local/metadata range is denied.
func hostFetchDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	deny := func(ip net.IP) error {
		if isHostileFetchIP(ip) {
			return fmt.Errorf("blocked: link-local/cloud-metadata address %s", ip)
		}
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if derr := deny(ip); derr != nil {
			return nil, derr
		}
		return d.DialContext(ctx, network, addr)
	}
	ips, lerr := net.DefaultResolver.LookupIPAddr(ctx, host)
	if lerr != nil {
		return nil, lerr
	}
	for _, a := range ips {
		if derr := deny(a.IP); derr != nil {
			return nil, derr
		}
	}
	return d.DialContext(ctx, network, addr)
}

// isHostileFetchIP reports whether ip is a link-local / known cloud-metadata
// address that an agent must never reach.
func isHostileFetchIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true // 169.254.0.0/16 (incl. 169.254.169.254), fe80::/10
	}
	switch ip.String() {
	case "100.100.100.200", // Alibaba metadata
		"192.0.0.192": // legacy Oracle/others
		return true
	}
	return false
}

// isLoopbackURL reports whether raw points at the local machine (used to decide
// when to attach the loopback caller-binding headers — never sent off-box).
func isLoopbackURL(raw string) bool {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	host := s
	if h, _, err := net.SplitHostPort(s); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func (st *hostState) netFetch(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req netFetchReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("url required"))
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}

	// Capability gate: minta `net:fetch:<url>` — broker prefix-match pakai
	// approved pattern `net:fetch:https://*.example.com`.
	if st.caps != nil && pluginID != "" {
		want := "net:fetch:" + req.URL
		if !st.caps(pluginID, want) {
			// Coba pattern matching via host-side glob (kalo approved
			// pakai `net:fetch:https://*.host.com`).
			if !matchesURLApproval(st, pluginID, req.URL) {
				return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+want))
			}
		}
	}

	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 60 * time.Second
	}
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if req.BodyB64 != "" {
		raw, derr := base64.StdEncoding.DecodeString(req.BodyB64)
		if derr != nil {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode body_base64: "+derr.Error()))
		}
		body = bytes.NewReader(raw)
	}
	httpReq, err := http.NewRequestWithContext(fetchCtx, req.Method, req.URL, body)
	if err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("build request: "+err.Error()))
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	// A2 isolation: for loopback self-API calls, override the caller identity with
	// the VERIFIED guest pluginID + attach the loopback secret. Set LAST so a
	// guest can't pre-set these — the tools/run handler then binds execution to
	// the real caller instead of trusting a guest-supplied ?id=.
	if pluginID != "" && isLoopbackURL(req.URL) {
		if secret := os.Getenv("FLOWORK_LOOPBACK_SECRET"); secret != "" {
			httpReq.Header.Set("X-Flowork-Caller", pluginID)
			httpReq.Header.Set("X-Flowork-Secret", secret)
		}
	}

	resp, ferr := st.http.Do(httpReq)
	if ferr != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("fetch: "+ferr.Error()))
	}
	defer resp.Body.Close()

	maxBytes := req.MaxRespByte
	if maxBytes <= 0 || maxBytes > 8<<20 {
		maxBytes = 4 << 20
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	truncated := false
	if len(respBody) > maxBytes {
		respBody = respBody[:maxBytes]
		truncated = true
	}

	headersOut := map[string]string{}
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			headersOut[k] = vs[0]
		}
	}
	if truncated {
		headersOut["X-Flowork-Truncated"] = "1"
	}

	result := netFetchResp{
		Status:  resp.StatusCode,
		Headers: headersOut,
		BodyB64: base64.StdEncoding.EncodeToString(respBody),
	}
	out, _ := json.Marshal(result)
	return writeJSONOrCrop(m, outPtr, outMax, out)
}

// matchesURLApproval — fallback ke pattern matching kalau broker exact-
// match miss. Approval format `net:fetch:<url-pattern-glob>`. Cuma cek
// satu approval at a time karena broker tidak expose list dari sini —
// kita ekstrak via Broker.Approved interface kalau di-passing nanti.
// Phase 7 MVP: glob via path.Match on host portion.
func matchesURLApproval(_ *hostState, _ string, _ string) bool {
	// Phase 7 MVP: broker IsApproved sudah handle prefix match. Kalau
	// approval persis (mis. `net:fetch:https://api.openai.com/v1/chat`),
	// kasus exact. Pattern matching dengan `*` di host akan ditangani
	// di Phase 11 saat marketplace UI generate approval rule canonical.
	return false
}

// helper supaya import "path" tidak idle.
var _ = path.Match

// execRun — implementasi `host_exec_run`. Plugin kirim request, host
// validate capability, jalanin command, balas response JSON di buffer.
func (st *hostState) execRun(ctx context.Context, m api.Module, reqPtr, reqLen, outPtr, outMax uint32) uint32 {
	pluginID := guestPluginID(ctx)

	reqBytes, ok := m.Memory().Read(reqPtr, reqLen)
	if !ok {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("cannot read request memory"))
	}
	var req execReq
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("decode request: "+err.Error()))
	}
	req.Binary = strings.TrimSpace(req.Binary)
	if req.Binary == "" {
		return writeJSONOrCrop(m, outPtr, outMax, errorJSON("binary required"))
	}

	// Capability gate. Kalau caller tidak punya `exec:<binary>`, denial
	// kembalikan ke plugin sebagai JSON error.
	if st.caps != nil && pluginID != "" {
		want := "exec:" + req.Binary
		if !st.caps(pluginID, want) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("capability denied: "+want))
		}
	}

	// Denylist on the RAW exec primitive. This host-func sits beside the builtin
	// `bash` tool but bypassed its denylist/env-scrub — so a self-asserted
	// `exec:<bin>` cap could run `rm -rf /`, `sudo`, `shutdown`, etc. Apply the
	// same conservative substring denylist here (defence-in-depth; the broker cap
	// + owner allowlist gate WHO may reach this at all).
	joined := strings.ToLower(req.Binary + " " + strings.Join(req.Args, " "))
	for _, p := range hostExecDeny {
		if strings.Contains(joined, p) {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("exec blocked: dangerous pattern "+p))
		}
	}

	// Cross-OS shell resolution: kalau plugin ngirim "bash" / "sh", host
	// pilih sendiri shell yang tepat. Lainnya treat as binary name di PATH.
	bin, args := resolveBinary(req.Binary, req.Args)

	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, bin, args...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	// Scrub env: never forward owner tokens/secrets into a guest-driven exec.
	cmd.Env = scrubHostExecEnv()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			return writeJSONOrCrop(m, outPtr, outMax, errorJSON("exec: "+err.Error()))
		}
	}
	resp := execResp{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	body, _ := json.Marshal(resp)
	return writeJSONOrCrop(m, outPtr, outMax, body)
}

// hostExecDeny — conservative substring denylist for the raw exec host-func
// (mirrors the builtin bash tool). Lower-cased compare.
var hostExecDeny = []string{
	"rm -rf /", "rm -rf /*", "rm -rf ~", "rm --no-preserve-root",
	":(){:|:&};:", "sudo ", "su -", "chmod 777", "chown -r", "mkfs",
	"dd if=/dev/zero", "dd if=/dev/random", "> /dev/sda", "> /dev/nvme",
	"shutdown", "reboot", "halt", "poweroff", "init 0", "init 6",
	"|sh", "| sh", "|bash", "| bash", "curl -s http", "wget -o -",
	"eval $", "eval `", "/etc/passwd", "/etc/shadow", "/etc/sudoers", ".ssh/id_rsa",
}

// scrubHostExecEnv — minimal env whitelist for guest-driven exec (no tokens).
func scrubHostExecEnv() []string {
	want := []string{"PATH", "HOME", "LANG", "LC_ALL", "TERM"}
	if runtime.GOOS == "windows" {
		want = []string{"SystemRoot", "Path", "TEMP", "TMP", "USERPROFILE"}
	}
	out := make([]string, 0, len(want))
	for _, k := range want {
		if v := os.Getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// resolveBinary — `bash`/`sh` di-route ke /bin/sh (POSIX) atau cmd.exe
// (Windows). Plugin ngirim `args` apa adanya; host tidak rewrite arg
// flags supaya plugin keep control.
func resolveBinary(name string, args []string) (string, []string) {
	switch strings.ToLower(name) {
	case "bash", "sh":
		if runtime.GOOS == "windows" {
			// Argumen pertama biasanya "-c"; cmd.exe pakai "/C". Plugin
			// sudah kirim args[0]="-c"; ganti supaya cocok dengan cmd.exe.
			out := append([]string{"/C"}, args[1:]...)
			return "cmd.exe", out
		}
		return "/bin/sh", args
	}
	return name, args
}

// writeJSONOrCrop tulis bytes ke memori plugin di range (ptr, max). Kalau
// payload lebih panjang dari max, crop. Return total bytes ditulis.
func writeJSONOrCrop(m api.Module, ptr, max uint32, body []byte) uint32 {
	if uint32(len(body)) > max {
		body = body[:max]
	}
	if !m.Memory().Write(ptr, body) {
		return 0
	}
	return uint32(len(body))
}

func errorJSON(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

// Bootstrap register WASI snapshot preview1 + flowork host module.
// Caller wajib panggil sekali sebelum Load() pertama.
func (r *Runtime) Bootstrap(ctx context.Context, caps CapsChecker, resolver InstanceResolver, interaction InteractionLogger, decision DecisionLogger, karma KarmaUpdater, slash SlashDispatcher) error {
	// WASI preview 1 — TinyGo target=wasi expect `_start` resolve via ini.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r.rt); err != nil {
		return fmt.Errorf("instantiate wasi: %w", err)
	}
	return r.registerFloworkHost(ctx, caps, resolver, interaction, decision, karma, slash)
}
