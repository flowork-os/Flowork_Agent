// Package safety — audit.go: append-only HPG violation audit log + karma
// penalty hook integration.
//
// Caller (kernel boot) wire `SetAuditSink()` ke implementasi yang persist
// violation ke `flowork-settings.sqlite.host_protection_audit` table.
//
// Pattern decoupling: HPG core ngga import settings DB langsung (anti import
// cycle). Caller inject sink + karma hook saat boot.
//
// Severity escalation:
//   - "critical" — instant Telegram notify Ayah + karma penalty -1.0
//   - "high"     — log + karma penalty -0.5
//   - "medium"   — log + karma penalty -0.2

package safety

import (
	"sync"
	"time"
)

// AuditEntry — 1 violation record.
type AuditEntry struct {
	Timestamp time.Time    `json:"ts"`
	Violation HPGViolation `json:"violation"`
	Caller    string       `json:"caller"`     // warga ID atau session ID
	BlockedBy string       `json:"blocked_by"` // "HPG"
}

// AuditSink — caller-injectable persistence.
type AuditSink interface {
	Record(entry AuditEntry) error
}

// KarmaHook — caller-injectable karma penalty handler.
type KarmaHook func(caller string, penalty float64, reason string)

// NotifyHook — caller-injectable Telegram notification untuk critical severity.
type NotifyHook func(message string)

var (
	auditMu     sync.RWMutex
	auditSink   AuditSink
	karmaHook   KarmaHook = func(caller string, penalty float64, reason string) {}
	notifyHook  NotifyHook = func(message string) {}
	memoryLog   []AuditEntry
	memoryLogCap = 1000
)

// SetAuditSink install persistence layer.
func SetAuditSink(sink AuditSink) {
	auditMu.Lock()
	defer auditMu.Unlock()
	auditSink = sink
}

// SetKarmaHook install karma penalty handler.
func SetKarmaHook(hook KarmaHook) {
	auditMu.Lock()
	defer auditMu.Unlock()
	if hook != nil {
		karmaHook = hook
	}
}

// SetNotifyHook install Telegram notify handler.
func SetNotifyHook(hook NotifyHook) {
	auditMu.Lock()
	defer auditMu.Unlock()
	if hook != nil {
		notifyHook = hook
	}
}

// RecordViolation — wire ini ke `SetCheckHook()` saat boot. Implements
// CheckHook signature untuk HPG main entry.
//
// Usage di kernel boot:
//
//	safety.SetAuditSink(myDBSink)
//	safety.SetKarmaHook(myKarmaEngine)
//	safety.SetNotifyHook(myTelegramBot)
//	safety.SetCheckHook(safety.RecordViolation)
func RecordViolation(v HPGViolation) {
	auditMu.RLock()
	sink := auditSink
	karma := karmaHook
	notify := notifyHook
	auditMu.RUnlock()

	entry := AuditEntry{
		Timestamp: time.Now(),
		Violation: v,
		Caller:    "unknown", // caller_context.go inject pattern future
		BlockedBy: "HPG",
	}

	// In-memory ring buffer (always available, untuk debug + recent stats)
	auditMu.Lock()
	if len(memoryLog) >= memoryLogCap {
		memoryLog = memoryLog[1:]
	}
	memoryLog = append(memoryLog, entry)
	auditMu.Unlock()

	// Persistent sink (best-effort, ngga panic kalau gagal)
	if sink != nil {
		_ = sink.Record(entry)
	}

	// Karma penalty per severity
	penalty := 0.0
	switch v.Severity {
	case "critical":
		penalty = -1.0
	case "high":
		penalty = -0.5
	case "medium":
		penalty = -0.2
	}
	if penalty < 0 {
		karma(entry.Caller, penalty, "HPG: "+v.Category+" violation")
	}

	// Critical → instant Telegram notify Ayah
	if v.Severity == "critical" {
		notify("🚨 HPG CRITICAL — tool=" + v.ToolName + " category=" + v.Category +
			" pattern=" + truncate(v.Pattern, 100) + " arg=" + v.Argument +
			" caller=" + entry.Caller + " ts=" + entry.Timestamp.Format(time.RFC3339))
	}
}

// RecentViolations — return last N audit entries (untuk debug + GUI dashboard).
func RecentViolations(n int) []AuditEntry {
	auditMu.RLock()
	defer auditMu.RUnlock()
	if n <= 0 || n > len(memoryLog) {
		n = len(memoryLog)
	}
	out := make([]AuditEntry, n)
	copy(out, memoryLog[len(memoryLog)-n:])
	return out
}

// ResetAuditLog — clear in-memory log (testing only).
func ResetAuditLog() {
	auditMu.Lock()
	defer auditMu.Unlock()
	memoryLog = nil
}
