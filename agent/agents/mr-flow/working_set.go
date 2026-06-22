// working_set.go — D18-P1: WORKING-SET (TUGAS AKTIF) persist lintas-sesi.
// Brain-logic di-EKSTRAK dari main.go (pola nano-modular spt recall_gate.go /
// recovery_capture.go): file terpisah FROZEN, main.go = wiring EDITABLE.
//
// MEKANISME: request SUBSTANTIF (bukan sapaan/ack — reuse isTrivialChat di recall_gate.go)
// → simpan ke kv (`memory_set`/tool_memory). Trivial chat GA ngubah (makasih ≠ tugas baru).
// main.go inject hasilnya BOTTOM-salient tiap turn → goal ga ke-scroll keluar window
// 16-turn / ga ilang walau restart. Reuse memory_get/set + parseAutoCont — 0 host-func, 0 tabel.
//
// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN brain-core — lihat lock/brain.md §7 (D18 working-memory) + §13. Unfreeze dulu buat edit.

package main

import "strings"

// d18TaskKey — kv key (tool_memory) tempat TUGAS AKTIF disimpan, persist lintas-sesi.
const d18TaskKey = "__d18_active_task"

// activeTaskFor — D18-P1: balikin TUGAS AKTIF yang persist. Kalau userText = request
// SUBSTANTIF (bukan sapaan/ack/filler), update kv jadi task itu (cap 240 rune). Trivial
// chat → kembaliin task lama (ga ke-wipe). Marker auto-continue di-strip via parseAutoCont
// (task = tugas ASLI, bukan marker). Best-effort: kv gagal → tetap balikin yg ada.
func activeTaskFor(userText string) string {
	active := fetchMemoryValue(d18TaskKey)
	if base, _ := parseAutoCont(strings.TrimSpace(userText)); base != "" && !isTrivialChat(base) {
		if len([]rune(base)) > 240 {
			base = string([]rune(base)[:240]) + "…"
		}
		if base != active {
			active = base
			runTool("memory_set", map[string]any{"key": d18TaskKey, "value": active})
		}
	}
	return active
}
