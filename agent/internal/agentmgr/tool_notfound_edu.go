// 🔒 FROZEN tools-core — Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// JANGAN edit tanpa unfreeze SADAR owner (di-hash KERNEL_FREEZE.md + chattr +i). Baca lock/tools.md DULU.
// Konten pelajaran error yg berevolusi ada di edu_errors_ext.go (CABANG, non-frozen) — edit di sana.
//
// tool_notfound_edu.go — FASE 3 SELF-EVOLVING: tool not-found → REKOMENDASI sepadan (drawer/semantic)
// → kalau bener-bener ga ada → ERROR-EDUKASI ngajarin tool_create (owner 2026-06-23).
//
// Alur (dari kasus mr-flow nyari tool yg ga ada):
//  1. Cari tool sepadan: localSuggest pakai NAMA-utuh + tiap TOKEN (pecah '_') → merge top-K distinct.
//     Ini "drawer" — tool tetep di registry, ke-discover walau ga ke-expose (cap maxExposedTools).
//  2. Selalu tutup dgn ajakan BIKIN tool sendiri pakai tool_create (lahir privat, langsung jalan buat
//     si pembuat, nanti bisa promote shared via Dewan). Flowork DIDESAIN tumbuh — di sinilah dia evolve.
//
// Dipanggil dari agentmgr.go handler tool-run (jalur LLM agent: agent-template POST /api/agents/tools/run
// → balik error ini → di-feed ke LLM sbg tool-result). NON-frozen.
package agentmgr

import (
	"sort"
	"strings"

	"flowork-gui/internal/toolsidecar"
)

// toolNotFoundEducation — pesan not-found yg MENGAJAR: rekomendasi sepadan + cara bikin tool sendiri.
func toolNotFoundEducation(name string) string {
	var b strings.Builder

	// DELETION-AWARE: kalau tool ini DULU ada tapi udah di-GC (tombstone), kasih sinyal beda — biar
	// agent SADAR dia mati (anti halu tool-hantu dari dream), bukan generic "salah nama".
	for _, t := range toolsidecar.Tombstones(toolsidecar.ToolsDir()) {
		if t == name {
			return "[PETUNJUK, bukan salahmu] tool '" + name + "' DULU ada tapi udah dihapus seleksi-alam (GC) — " +
				"kebanyakan error (API-nya mungkin berubah) atau lama nganggur. JANGAN maksa akses bangkainya, " +
				"hasilnya ga bakal balik. Kalau fungsinya masih kamu butuh, BIKIN versi baru via tool_create " +
				"(sesuaiin keadaan terkini). Doktrin: ERR_TOOL_GC_REMOVED."
		}
	}

	b.WriteString("[PETUNJUK, bukan salahmu] tool '" + name + "' belum terdaftar — JANGAN ngarang hasilnya. ")

	if sugs := suggestSimilarTools(name, 4); len(sugs) > 0 {
		parts := make([]string, 0, len(sugs))
		for _, s := range sugs {
			d := s.Description
			if len(d) > 60 {
				d = d[:60] + "…"
			}
			parts = append(parts, s.Name+" ("+d+")")
		}
		b.WriteString("Mungkin maksudmu salah satu ini (pakai apa adanya, atau cek detail via tool_search): " +
			strings.Join(parts, "; ") + ". ")
	} else {
		b.WriteString("Ga ada tool sepadan di laci — coba tool_search <kata-kunci> dulu buat mastiin. ")
	}

	// ERROR-EDUKASI: ajarin bikin tool sendiri (inti self-evolving). Selalu muncul — kalau emang ga ada
	// yg cocok, jalan keluarnya BIKIN, bukan ngarang.
	b.WriteString("Kalau emang GA ADA yang cocok, kamu boleh BIKIN tool sendiri pakai tool_create — " +
		"lahir PRIVAT & langsung jalan buat kamu, nanti bisa naik jadi SHARED kalau lolos Dewan. " +
		`Format: tool_create{"name":"snake_case","description":"...","code":"<badan Go, balikin: return <hasil>, \"\">"}. ` +
		"Tool jalan sbg proses terpisah (stdin args → stdout output) — murni & terisolasi, tanpa os/exec/jaringan.")
	return b.String()
}

// suggestSimilarTools — localSuggest pakai nama-utuh + tiap token (pecah '_'), merge skor, top-K distinct.
// Token-split bikin "rephrase_text" nemu "reverse_text"/"text_hash" lewat token "text" (substring murni
// ga bakal nyangkut). Skor token di-diskon 0.5 biar match nama-utuh tetep menang.
func suggestSimilarTools(name string, k int) []suggestEntry {
	q := strings.ToLower(strings.TrimSpace(name))
	merged := map[string]suggestEntry{}
	add := func(es []suggestEntry, weight float64) {
		for _, e := range es {
			if e.Name == name { // jangan saranin dirinya sendiri (kalau kebetulan ada)
				continue
			}
			cur, ok := merged[e.Name]
			e.Score *= weight
			if !ok || e.Score > cur.Score {
				merged[e.Name] = e
			}
		}
	}
	if q != "" {
		add(localSuggest(q, k*2), 1.0)
	}
	for _, tok := range strings.FieldsFunc(q, func(r rune) bool { return r == '_' || r == '-' || r == ' ' }) {
		if len(tok) < 3 { // skip token cemen ("id", "to", "of")
			continue
		}
		add(localSuggest(tok, k), 0.5)
	}
	out := make([]suggestEntry, 0, len(merged))
	for _, e := range merged {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > k {
		out = out[:k]
	}
	return out
}
