// 📄 Dok: FLowork_os/lock/group.md
//
// groupsapi_seam.go — MEKANISME + SWITCH-READER BEKU (POLA-A/B) buat orchestrator.go (frozen).
// Pisahan dari groupsapi_ext.go (non-frozen, titik REGISTRASI hook): mekanisme + default aman ADA
// DI SINI biar inti beku self-sufficient (delete-test §6.4: hapus file registrasi → groupSyncHooks
// kosong + switch baca ENV/default → build OK). Switch sejati = ENV (FLOWORK_GROUP_SLASH /
// FLOWORK_ORCHESTRATOR), jadi bekuin fungsi pembacanya TIDAK ngurangin konfigurabilitas.
//
// Hook sync BARU: JANGAN edit file ini — daftar via init(){ RegisterGroupSyncHook(...) } di SIBLING.
package groupsapi

import (
	"os"
	"strings"
)

// slashPushEnabled — SAKLAR master push daftar group ke menu slash Telegram (setMyCommands).
// DEFAULT MATI (owner 2026-06-23: andelin kesadaran Mr.Flow). Idupin: ENV FLOWORK_GROUP_SLASH=1
// (TANPA buka orchestrator.go frozen).
func slashPushEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_GROUP_SLASH"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

// effectiveOrchestratorID — id agent orchestrator. SWITCH abadi (Rule 7): ENV FLOWORK_ORCHESTRATOR,
// default "mr-flow". orchestrator.go (frozen) init `var OrchestratorID = effectiveOrchestratorID()`
// → migrasi orchestrator cukup set ENV. Lihat lock/mrflow.md §6b.
func effectiveOrchestratorID() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ORCHESTRATOR")); v != "" {
		return v
	}
	return "mr-flow"
}

// GroupSyncHook — hook OPSIONAL tiap SyncToOrchestrator, dapet daftar group LIVE ("id|command|desc").
type GroupSyncHook func(parts []string)

var groupSyncHooks []GroupSyncHook

// RegisterGroupSyncHook — daftarin hook (panggil dari init() file SIBLING non-frozen).
func RegisterGroupSyncHook(fn GroupSyncHook) {
	if fn != nil {
		groupSyncHooks = append(groupSyncHooks, fn)
	}
}

// runGroupSyncHooks — sebar roster LIVE ke tiap hook terdaftar. Dipanggil SyncToOrchestrator
// (frozen). Hook yg error ga boleh ngerusak sync (recover).
func runGroupSyncHooks(parts []string) {
	for _, fn := range groupSyncHooks {
		func() {
			defer func() { _ = recover() }()
			fn(parts)
		}()
	}
}
