// Package audit — kernel-wide append-only audit trail per PRINSIP P-16.
//
// Sebelumnya audit fragmented (workspace pakai stderr, tool register cuma
// log.Printf, browser ngga ada sama sekali). BUG-007 + BUG-080 + BUG-094.
//
// Sekarang: shared package dengan satu file per kategori (workspace.jsonl,
// tool.jsonl, browser.jsonl, capability.jsonl). Open-append-close per write
// — Windows TempDir cleanup safe + simple shutdown durability.

package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/flowork/kernel/kernel/path"
)

// Entry generic schema. Caller pakai package-spesifik (AppendBrowser, dll)
// supaya field konsisten.
type Entry struct {
	TS      time.Time      `json:"ts"`
	Kind    string         `json:"kind"` // "workspace" | "tool" | "browser" | "capability"
	WargaID string         `json:"warga_id"`
	Action  string         `json:"action"`
	Target  string         `json:"target,omitempty"` // path / url / tool name
	Result  string         `json:"result"`
	Detail  map[string]any `json:"detail,omitempty"`
}

var auditMu sync.Mutex

func resolveDir() (string, error) {
	dir := os.Getenv("KERNEL_AUDIT_DIR")
	if dir == "" {
		d, err := path.Resolve("audit")
		if err != nil {
			return "", err
		}
		dir = d
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// appendEntry write entry ke file <dir>/<kind>.jsonl. Open-append-close
// per write supaya Windows test cleanup ngga deadlock.
func appendEntry(e Entry) {
	if e.Kind == "" {
		e.Kind = "generic"
	}
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[AUDIT-FALLBACK] marshal err: %v\n", err)
		return
	}

	auditMu.Lock()
	defer auditMu.Unlock()

	dir, err := resolveDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[AUDIT-FALLBACK] dir err=%v entry=%s\n", err, string(line))
		return
	}
	fpath := filepath.Join(dir, e.Kind+".jsonl")
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[AUDIT-FALLBACK] open err=%v entry=%s\n", err, string(line))
		return
	}
	defer f.Close()

	_, _ = f.Write(line)
	_, _ = f.Write([]byte("\n"))
}

// AppendBrowser record browser op. action: "navigate"|"click"|"extract"|
// "screenshot"|"eval". target: URL atau selector atau truncated js_code.
func AppendBrowser(wargaID, action, target, result string, detail map[string]any) {
	appendEntry(Entry{
		Kind: "browser", WargaID: wargaID, Action: action,
		Target: target, Result: result, Detail: detail,
	})
}

// AppendTool record tool register/execute op.
func AppendTool(wargaID, action, toolName, result string, detail map[string]any) {
	appendEntry(Entry{
		Kind: "tool", WargaID: wargaID, Action: action,
		Target: toolName, Result: result, Detail: detail,
	})
}

// AppendCapabilityCheck record capability_check decision.
func AppendCapabilityCheck(wargaID, capability, result string) {
	appendEntry(Entry{
		Kind: "capability", WargaID: wargaID, Action: "check",
		Target: capability, Result: result,
	})
}
