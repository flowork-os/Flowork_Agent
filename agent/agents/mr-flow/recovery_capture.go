// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN brain-core — jangan edit tanpa unfreeze owner. Arsitektur & alasan: lihat lock/brain.md + roadmap_brain.md #1
//
// recovery_capture.go — D32 INC-2 brain-pathway: AUTO-CAPTURE error→recovery
// (self-learning recovery instinct). Di-EKSTRAK dari main.go biar logic-brain
// ke-FREEZE terpisah; main.go = orkestrator/list (nano-modular, tetap editable)
// yang cuma manggil captureRecovery() di tool-loop.
//
// Pas tool ERROR lalu tool yg SAMA SUKSES dalam loop turn = agent berhasil recover
// → mistake_log("WHEN <tool> <kelas> -> recovered"). TITLE stabil (tool+kelas) =
// dedup + hit_count; CONTENT "WHEN..->.." → recovery-instinct via
// PromoteRecurringMistakes (INC-1) pas hit≥3 (gerbang repetisi anti-degenerasi).
// Kelas error BEBAS path/data owner (privasi D8). selfID()/runTool() = package main
// (di main.go). TinyGo-safe (substring, no regexp).

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// recoveryCaptureSkip — tool meta/recall/notify yg error→sukses-nya BUKAN "recovery"
// bermakna (ga usah jadi instinct). Action tool (file/shell/task/codemap/web/...) di-capture.
var recoveryCaptureSkip = map[string]bool{
	"mistake_log": true, "graph_recall": true, "brain_search": true,
	"brain_search_shared": true, "instinct_recall": true, "tool_search": true,
	"telegram_send": true, "ScheduleWakeup": true, "now": true,
	"memory_get": true, "interaction_recall": true,
}

// toolErrClass — hasil tool error host ({"error":...}) → KELAS error ringkas yg BEBAS
// path/identifier owner. Sukses ({"ok":true}) / non-JSON (mis. teks file_read) → "".
func toolErrClass(result string) string {
	t := strings.TrimSpace(result)
	if !strings.HasPrefix(t, "{") {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(t), &m) != nil {
		return ""
	}
	if ok, _ := m["ok"].(bool); ok {
		return "" // sukses eksplisit
	}
	es, _ := m["error"].(string)
	es = strings.ToLower(strings.TrimSpace(es))
	if es == "" {
		return ""
	}
	switch {
	case strings.Contains(es, "not found"), strings.Contains(es, "no such"), strings.Contains(es, "tidak ada"), strings.Contains(es, "404"):
		return "not-found"
	case strings.Contains(es, "permission"), strings.Contains(es, "denied"), strings.Contains(es, "ditolak"):
		return "permission"
	case strings.Contains(es, "timeout"), strings.Contains(es, "timed out"), strings.Contains(es, "deadline"):
		return "timeout"
	case strings.Contains(es, "already exists"), strings.Contains(es, "sudah ada"):
		return "already-exists"
	case strings.Contains(es, "invalid"), strings.Contains(es, "parse"), strings.Contains(es, "syntax"), strings.Contains(es, "unmarshal"), strings.Contains(es, "required"):
		return "invalid-input"
	case strings.Contains(es, "dispatch"), strings.Contains(es, "tool http"), strings.Contains(es, "capability"), strings.Contains(es, "denied by cap"):
		return "blocked"
	default:
		return "error"
	}
}

// captureRecovery — seen[tool] = kelas error tool yg BELUM ke-recover dalam loop turn
// ini. Pas tool yg SAMA sukses setelah error → mistake_log. Side-effect only (ga
// sentuh msgs/hasil). Dipanggil dari tool-loop di main.go.
func captureRecovery(tool, result string, seen map[string]string) {
	if recoveryCaptureSkip[tool] {
		return
	}
	if cls := toolErrClass(result); cls != "" {
		seen[tool] = cls // tool ini lagi error, tunggu apakah nanti sukses
		return
	}
	cls, had := seen[tool]
	if !had {
		return // sukses tanpa error sebelumnya = bukan recovery
	}
	delete(seen, tool)
	_ = runTool("mistake_log", map[string]any{
		"category":       "workflow",
		"title":          "recovery: " + tool + "/" + cls,
		"content":        "WHEN " + tool + " gagal (" + cls + ") -> RECOVERED: agent berhasil di percobaan berikutnya (perbaiki argumen / ganti pendekatan).",
		"context_origin": "auto-recovery",
	})
	fmt.Fprintf(os.Stderr, "[%s] D32 INC-2 recovery captured: %s/%s\n", selfID(), tool, cls)
}
