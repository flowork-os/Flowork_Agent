// groupsapi_ext.go — CABANG (extension point) NON-FROZEN buat file groupsapi yang
// FROZEN (orchestrator.go, groupsapi.go, dst).
//
// ⚖️ ATURAN ABADI (owner Mr.Dev, 2026-06-23): file yang udah di-FREEZE TIDAK BOLEH
// dibuka lagi buat nambah filtur. Tiap switch/hook/filtur baru soal group masuk SINI,
// bukan ngedit file frozen. File frozen cuma MANGGIL fungsi di sini → ga pernah
// disentuh lagi. Ini realisasi perintah owner: "kasih cabang file/switch biar file
// frozen ngak akan pernah dibuka lagi".
//
// 📖 WAJIB BACA: /home/mrflow/Documents/FLowork_os/lock/group.md sebelum ngutak-atik
// apapun soal group (cara kerja, filtur, cara bikin group — lengkap di situ).
package groupsapi

import (
	"os"
	"strings"
)

// slashPushEnabled — SAKLAR master buat push daftar group ke menu slash Telegram
// (setMyCommands). DEFAULT MATI: owner mutusin 2026-06-23 buang slash, andelin
// KESADARAN Mr.Flow (routing lewat task_list/task_run). Pas MATI, SyncToOrchestrator
// kirim daftar KOSONG → menu Telegram tetep bersih walau ada operasi group.
//
// Mau idupin slash lagi (mis. kalau migrasi mr-flow-next kelar)? Set env
// FLOWORK_GROUP_SLASH=1 — TANPA buka file frozen orchestrator.go.
func slashPushEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_GROUP_SLASH"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

// effectiveOrchestratorID — id agent orchestrator (yang punya kv "groups" + ask_group).
// SWITCH abadi (Rule 7): ENV FLOWORK_ORCHESTRATOR, default "mr-flow" (orchestrator LIVE;
// mr-flow-next belum ke-deploy — owner 2026-06-25 revert ke akar). orchestrator.go (frozen)
// init `var OrchestratorID = effectiveOrchestratorID()` → migrasi orchestrator nanti cukup
// set ENV, file frozen GA dibuka lagi. SATU switch dgn host (FLOWORK_ORCHESTRATOR). Lihat lock/mrflow.md §6b.
func effectiveOrchestratorID() string {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_ORCHESTRATOR")); v != "" {
		return v
	}
	return "mr-flow"
}

// GroupSyncHook — hook OPSIONAL yang dijalanin tiap SyncToOrchestrator, dapet daftar
// group LIVE ("id|command|desc"). Target sync masa depan (menu Discord/WhatsApp,
// registry eksternal, metrik, dll) daftar lewat sini → orchestrator.go (frozen) GA
// PERNAH diubah lagi.
type GroupSyncHook func(parts []string)

var groupSyncHooks []GroupSyncHook

// RegisterGroupSyncHook — daftarin hook (panggil dari init() file NON-frozen, mis.
// modul channel baru). Daftar pas startup; sekali daftar, jalan tiap sync.
func RegisterGroupSyncHook(fn GroupSyncHook) {
	if fn != nil {
		groupSyncHooks = append(groupSyncHooks, fn)
	}
}

// runGroupSyncHooks — sebar roster LIVE ke tiap hook terdaftar. Dipanggil
// SyncToOrchestrator (frozen). Hook yang error ga boleh ngerusak sync (recover).
func runGroupSyncHooks(parts []string) {
	for _, fn := range groupSyncHooks {
		func() {
			defer func() { _ = recover() }()
			fn(parts)
		}()
	}
}
