// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-02
// Audit rilis 2026-06-14 (§38 AI Agent): aman. resolvePowerCmdFor(goos,action) bisa dites lintas-OS;
// Android dibedakan (butuh root) dgn error edukatif. linux=RasPi/STB. Cap exec:power + ARM + dry-run.
// Reason: Operator power tool. E2E verified (LLM→system_power→dry-run→audit).
//   3 controls: cap exec:power + ARM switch (FLOWORK_POWER_ARMED) + audit.
//   argv exec (no shell). Cancellable timer with recover(). Extend (new
//   actions / persistent arm / cancel UI) → tambah file baru, JANGAN modify ini.
//
// system_power.go — Section 11 extension: host power control tool.
//
// PURPOSE:
//   Sanctioned, capability-gated path for an OPERATOR agent to control the
//   host computer's power state (shutdown / reboot / suspend / lock / logout)
//   + cancel a pending action. This is the ONLY allowed route for power ops —
//   the generic `bash` tool denylists shutdown/reboot/poweroff/halt on purpose
//   (shell.go), and the HPG baseline blocks them as raw commands. system_power
//   is the deliberate exception, fenced by three controls:
//
//     1. Capability gate `exec:power` — broker only approves agents whose
//        manifest.capabilities_required lists it (the operator agent), so a
//        normal chat agent can NEVER trigger power ops.
//     2. ARM switch — default behaviour is DRY-RUN: the command is resolved,
//        audited, and returned, but NOT executed. Real execution requires the
//        host env `FLOWORK_POWER_ARMED` to be truthy. This keeps dev/test on a
//        live machine safe, and makes "go live" an explicit owner decision.
//     3. Audit — every call (dry-run or armed) appends to the agent audit log.
//
//   Multi-OS: Linux (systemctl/loginctl — polkit, no sudo on desktop), macOS
//   (osascript/pmset), Windows (shutdown.exe/rundll32). No shell — argv exec,
//   so no injection surface. A cancel window is real: the delay runs in-process
//   and `action:"cancel"` aborts a still-pending action.

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/tools"
)

// systemPowerTool — control host power state. Cap-gated `exec:power`.
type systemPowerTool struct{}

func (systemPowerTool) Name() string       { return "system_power" }
func (systemPowerTool) Capability() string { return "exec:power" }
func (systemPowerTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Kontrol power HOST: shutdown|reboot|suspend|lock|logout|cancel. Butuh cap exec:power (operator tepercaya), tiap aksi di-audit. Eksekusi NYATA cuma kalau ARMED (env FLOWORK_POWER_ARMED), else dry-run. WAJIB konfirmasi user sebelum shutdown/reboot.",
		Params: []tools.Param{
			{Name: "action", Type: tools.ParamString, Description: "shutdown | reboot | suspend | lock | logout | cancel", Required: true},
			{Name: "delay_seconds", Type: tools.ParamInt, Description: "jeda sebelum eksekusi (window cancel); default 10, max 3600"},
			{Name: "reason", Type: tools.ParamString, Description: "alasan singkat, masuk audit log"},
		},
		Returns: "{status, action, delay_seconds, armed, command, message}",
	}
}

// validPowerActions — whitelist. Anything else is rejected.
var validPowerActions = map[string]bool{
	"shutdown": true,
	"reboot":   true,
	"suspend":  true,
	"lock":     true,
	"logout":   true,
	"cancel":   true,
}

// pending power action — single in-flight timer guarded by a mutex so a
// `cancel` can abort a still-waiting shutdown. Process-wide (one host).
var (
	powerMu      sync.Mutex
	powerTimer   *time.Timer
	powerPending string // human description of what's queued ("" = nothing)
)

func powerArmed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_POWER_ARMED"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func (systemPowerTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := args["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	if !validPowerActions[action] {
		return tools.Result{}, fmt.Errorf("system_power: invalid action %q (want shutdown|reboot|suspend|lock|logout|cancel)", action)
	}

	store, _ := tools.FromStore(ctx)
	caller := tools.FromCaller(ctx)
	reason, _ := args["reason"].(string)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "(no reason given)"
	}

	// ── cancel: abort a pending action ──────────────────────────────────────
	if action == "cancel" {
		powerMu.Lock()
		had := powerTimer != nil
		desc := powerPending
		if powerTimer != nil {
			powerTimer.Stop()
			powerTimer = nil
			powerPending = ""
		}
		powerMu.Unlock()
		writePowerAudit(store, caller, action, reason, "(cancel)", powerArmed(), agentdb.AuditSevInfo)
		if had {
			return tools.Result{Output: map[string]any{
				"status":  "cancelled",
				"action":  "cancel",
				"message": "Pending power action cancelled: " + desc,
			}}, nil
		}
		return tools.Result{Output: map[string]any{
			"status":  "nothing_pending",
			"action":  "cancel",
			"message": "No pending power action to cancel.",
		}}, nil
	}

	// ── delay clamp ─────────────────────────────────────────────────────────
	delay := 10
	if v, ok := args["delay_seconds"].(float64); ok {
		delay = int(v)
	}
	if v, ok := args["delay_seconds"].(int); ok {
		delay = v
	}
	if delay < 0 {
		delay = 0
	}
	if delay > 3600 {
		delay = 3600
	}

	// ── resolve OS-specific argv ────────────────────────────────────────────
	argv, err := resolvePowerCmd(action)
	if err != nil {
		return tools.Result{}, err
	}
	cmdStr := strings.Join(argv, " ")
	armed := powerArmed()

	// Audit BEFORE executing (a shutdown may take the host down mid-log).
	sev := agentdb.AuditSevWarning
	if action == "lock" || action == "logout" {
		sev = agentdb.AuditSevInfo
	}
	writePowerAudit(store, caller, action, reason, cmdStr, armed, sev)

	// ── DRY-RUN (not armed): resolve + audit, but do NOT execute ────────────
	if !armed {
		return tools.Result{
			Output: map[string]any{
				"status":        "dry_run",
				"action":        action,
				"delay_seconds": delay,
				"armed":         false,
				"command":       cmdStr,
				"message":       fmt.Sprintf("DRY-RUN: would run %q in %ds. Host is NOT armed — set FLOWORK_POWER_ARMED=1 to enable real power control.", cmdStr, delay),
			},
			Note: "host not armed (FLOWORK_POWER_ARMED unset) — no action taken",
		}, nil
	}

	// ── ARMED: schedule real execution after delay (cancellable) ────────────
	desc := fmt.Sprintf("%s (in %ds) — %s", action, delay, reason)
	powerMu.Lock()
	if powerTimer != nil { // replace any previous pending action
		powerTimer.Stop()
	}
	powerPending = desc
	powerTimer = time.AfterFunc(time.Duration(delay)*time.Second, func() {
		defer func() {
			if r := recover(); r != nil {
				writePowerAudit(store, caller, action, fmt.Sprintf("panic during exec: %v", r), cmdStr, true, agentdb.AuditSevError)
			}
		}()
		powerMu.Lock()
		powerTimer = nil
		powerPending = ""
		powerMu.Unlock()
		c := exec.Command(argv[0], argv[1:]...)
		c.Env = os.Environ() // power tools need the real session env (DBUS/XDG)
		if runErr := c.Run(); runErr != nil {
			// Best-effort: the machine may already be going down. Log if we can.
			writePowerAudit(store, caller, action, "exec failed: "+runErr.Error(), cmdStr, true, agentdb.AuditSevError)
		}
	})
	powerMu.Unlock()

	return tools.Result{Output: map[string]any{
		"status":        "scheduled",
		"action":        action,
		"delay_seconds": delay,
		"armed":         true,
		"command":       cmdStr,
		"message":       fmt.Sprintf("Power action %q scheduled in %ds. Call system_power with action=cancel within the window to abort.", action, delay),
	}}, nil
}

// resolvePowerCmd — map (action, OS) → argv. No shell; argv only.
func resolvePowerCmd(action string) ([]string, error) {
	return resolvePowerCmdFor(runtime.GOOS, action)
}

// resolvePowerCmdFor — varian OS-param (bisa dites lintas platform tanpa spawn).
// Linux mencakup Raspberry Pi & STB berbasis Linux (systemctl). Android sengaja
// TIDAK didukung default: shutdown butuh root (`svc power shutdown`/`reboot -p`),
// jadi "dibedakan" per desain owner — errornya edukatif (lihat di bawah), bukan diam.
func resolvePowerCmdFor(goos, action string) ([]string, error) {
	switch goos {
	case "linux":
		switch action {
		case "shutdown":
			return []string{"systemctl", "poweroff"}, nil
		case "reboot":
			return []string{"systemctl", "reboot"}, nil
		case "suspend":
			return []string{"systemctl", "suspend"}, nil
		case "lock":
			return []string{"loginctl", "lock-session"}, nil
		case "logout":
			user := os.Getenv("USER")
			if user == "" {
				return nil, fmt.Errorf("system_power: $USER kosong jadi belum bisa logout. Petunjuk: jalankan Flowork di sesi login user (bukan service tanpa env), atau set env USER")
			}
			return []string{"loginctl", "terminate-user", user}, nil
		}
	case "darwin":
		switch action {
		case "shutdown":
			return []string{"osascript", "-e", `tell app "System Events" to shut down`}, nil
		case "reboot":
			return []string{"osascript", "-e", `tell app "System Events" to restart`}, nil
		case "suspend":
			return []string{"pmset", "sleepnow"}, nil
		case "lock":
			return []string{"pmset", "displaysleepnow"}, nil
		case "logout":
			return []string{"osascript", "-e", `tell app "System Events" to log out`}, nil
		}
	case "windows":
		switch action {
		case "shutdown":
			return []string{"shutdown.exe", "/s", "/t", "0"}, nil
		case "reboot":
			return []string{"shutdown.exe", "/r", "/t", "0"}, nil
		case "suspend":
			return []string{"rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0"}, nil
		case "lock":
			return []string{"rundll32.exe", "user32.dll,LockWorkStation"}, nil
		case "logout":
			return []string{"shutdown.exe", "/l"}, nil
		}
	case "android":
		// Dibedakan per desain: Android non-root tidak bisa shutdown program.
		return nil, fmt.Errorf("system_power di Android dibedakan: shutdown/reboot butuh ROOT (`svc power shutdown` / `reboot -p`). Petunjuk: di Android pakai fitur lain (notifikasi/akun), kontrol daya OS diserahkan ke user — ini normal, bukan kesalahan lo")
	}
	return nil, fmt.Errorf("system_power: aksi %q belum dipetakan untuk OS %q. Petunjuk: OS yang didukung penuh = linux (termasuk RasPi/STB), darwin (macOS), windows. Kalau ini OS baru, perlu tambah cabang di resolvePowerCmdFor", action, goos)
}

// writePowerAudit — best-effort append to agent audit log.
func writePowerAudit(store *agentdb.Store, caller, action, reason, cmdStr string, armed bool, sev string) {
	if store == nil {
		return
	}
	detail, _ := json.Marshal(map[string]any{
		"tool":    "system_power",
		"action":  action,
		"reason":  reason,
		"command": cmdStr,
		"armed":   armed,
	})
	_, _ = store.AppendAudit(agentdb.AuditEntry{
		EventType:  agentdb.EventToolCall,
		Severity:   sev,
		Actor:      caller,
		DetailJSON: string(detail),
	})
}
