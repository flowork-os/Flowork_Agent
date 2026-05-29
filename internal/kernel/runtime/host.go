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
	"net/http"
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

// hostState dibawa per-instantiation supaya host_exec_run tau pluginID
// pemanggil. Setiap instance plugin diberi ApiContext dengan pluginID
// melalui WithGuestPluginID() saat Call.
type hostState struct {
	caps        CapsChecker
	resolver    InstanceResolver
	interaction InteractionLogger
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
func (r *Runtime) registerFloworkHost(ctx context.Context, caps CapsChecker, resolver InstanceResolver, interaction InteractionLogger) error {
	st := &hostState{
		caps:        caps,
		resolver:    resolver,
		interaction: interaction,
		http:        &http.Client{Timeout: 120 * time.Second},
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
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("instantiate flowork host: %w", err)
	}
	return nil
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
func (r *Runtime) Bootstrap(ctx context.Context, caps CapsChecker, resolver InstanceResolver, interaction InteractionLogger) error {
	// WASI preview 1 — TinyGo target=wasi expect `_start` resolve via ini.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r.rt); err != nil {
		return fmt.Errorf("instantiate wasi: %w", err)
	}
	return r.registerFloworkHost(ctx, caps, resolver, interaction)
}
