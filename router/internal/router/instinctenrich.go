// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without explicit owner (Mr.Dev) approval.
// Owner: Aola Sahidin (Mr.Dev). Repo: https://github.com/flowork-os/Flowork-OS
// Frozen: 2026-06-25 (chattr +i + hash di KERNEL_FREEZE.md). Sibling mistakeenrich.go.
//
// ⚠️ AI MANAPUN (termasuk pasca-compact): JANGAN extend file ini dengan ngedit-nya.
//   Mau ganti cara MILIH insting (rank semantic / scoping #6 / boost domain)?
//   → RegisterInstinctSelector() dari instinctenrich_ext.go (NON-frozen). 99% perluasan
//   GA perlu buka freeze. Buka frozen = keputusan sadar + izin owner (CARAFREEZE.MD).
//   Tuning runtime: ENV FLOWORK_INSTINCT_INJECT / _MAX. Tumbuh awareness: tambah drawer.
//
// instinctenrich.go — INSTINCT INJECTION (sibling mistakeenrich.go).
//
// Logika sengaja simpel + fails-open. Mirror PERSIS pola antibodi.
//
// AKAR MASALAH (owner 2026-06-25, "mobil mewah tapi ga tau naiknya"):
//   Doktrin (maybeInjectConstitution) + antibodi (maybeInjectAntibodies) + fakta
//   (agent fetchAutoRecall) di-PAKSA masuk prompt tiap turn — proaktif. TAPI INSTING
//   (WHEN→THEN: kapan pakai tool/fitur) cuma PULL-ONLY (tool instinct_recall) → buat
//   tau dia HARUS recall, agent butuh insting = telur-ayam → kapabilitas parkir,
//   agent "ga sadar kapan manggil". File ini nutup pipa itu: maksa insting relevan
//   masuk prompt (prinsip owner: "jangan ngarep model manggil sendiri — PAKSA injeksi").
//
// Ranking: token-overlap × importance (deterministik, NO vindex — jalan walau
// brain.vindex belum di-rebuild). Fails open: brain mati / kosong / error → skip,
// request tetap normal. Ga pernah bikin request gagal.
//
// SWITCH (Rule 7 — ubah TANPA buka kode):
//   FLOWORK_INSTINCT_INJECT=0       → matiin total (default ON)
//   FLOWORK_INSTINCT_INJECT_MAX=N   → cap jumlah insting/inject (default 3, 0=mati, max 10)

package router

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

const (
	// instinctInjectMaxDefault — hard cap default (anti over-prompt, sejajar antibodi MAX 3).
	instinctInjectMaxDefault = 3
	// instinctScanLimit — berapa insting di-scan dari brain sebelum di-rank.
	instinctScanLimit = 300
	// instinctFoundationImp — importance >= ini = insting FONDASI: tetap kandidat
	// walau overlap 0 (kayak antibodi universal-karma). Di bawah ini wajib relevan.
	instinctFoundationImp = 7.0
)

// instinctInjectEnabled — switch ENV. Default ON. "0" → matiin.
func instinctInjectEnabled() bool {
	return strings.TrimSpace(os.Getenv("FLOWORK_INSTINCT_INJECT")) != "0"
}

// instinctInjectMax — cap jumlah insting yg disuntik. ENV override (0..10).
func instinctInjectMax() int {
	if v := strings.TrimSpace(os.Getenv("FLOWORK_INSTINCT_INJECT_MAX")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10 {
			return n
		}
	}
	return instinctInjectMaxDefault
}

// instinctSelectHook — SWITCH (Rule 7). Kalau di-set (oleh instinctenrich_ext.go,
// NON-frozen) → GANTI logika seleksi default. nil = pakai rankInstincts (token-overlap).
var instinctSelectHook func(all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer

// instinctSelectHookCtx — varian CTX-AWARE (#3 RI-5): selector dapet ctx → bisa baca
// caller agent id (AgentIDFromContext) buat scope insting by-peran. Kalau di-set, MENANG
// atas hook lama. Di-register dari sibling NON-frozen instinctenrich_ext2.go.
var instinctSelectHookCtx func(ctx context.Context, all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer

// RegisterInstinctSelectorCtx — daftarin selector CTX-AWARE (terima ctx) TANPA buka freeze.
// Dipakai #3 scoped-instinct (sibling _ext2). Kalau di-set, dipakai duluan (lihat call-site).
func RegisterInstinctSelectorCtx(fn func(ctx context.Context, all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer) {
	instinctSelectHookCtx = fn
}

// RegisterInstinctSelector — daftarin pemilih insting custom TANPA buka freeze.
// fn terima SEMUA kandidat + query + cap → balikin insting final. Seam buat evolusi:
//   - rank SEMANTIC pas brain.vindex idup (RI-1) ganti token-overlap,
//   - scoping per-agent/model (#6: agent LUAR non-flowork skip room=instinct_tool),
//   - filter/boost domain custom.
//
// Dipanggil dari instinctenrich_ext.go init(). nil → perilaku default ga berubah.
func RegisterInstinctSelector(fn func(all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer) {
	instinctSelectHook = fn
}

// maybeInjectInstinct — dipanggil dari dispatcher SETELAH maybeInjectAntibodies.
// Mutate req.Messages in-place (nambah 1 system message insting). Best-effort.
func maybeInjectInstinct(ctx context.Context, req *OpenAIRequest, settings *store.Settings) {
	if !instinctInjectEnabled() {
		return
	}
	max := instinctInjectMax()
	if max == 0 {
		return
	}
	if settings == nil || !settings.Brain.Enabled {
		return
	}
	if settings.Brain.DBPath != "" {
		brain.SetDBPath(settings.Brain.DBPath)
	}
	if !brain.Available() {
		return
	}
	query := lastUserText(req.Messages)
	if query == "" {
		return
	}
	all, err := brain.ListInstinctDrawers(ctx, instinctScanLimit)
	if err != nil || len(all) == 0 {
		return
	}
	// SWITCH: ext bisa ganti seleksi (semantic/scoping) TANPA buka freeze; default token-overlap.
	var ins []brain.InstinctDrawer
	switch {
	case instinctSelectHookCtx != nil:
		ins = instinctSelectHookCtx(ctx, all, query, max) // #3 ctx-aware (scoped) menang
	case instinctSelectHook != nil:
		ins = instinctSelectHook(all, query, max)
	default:
		ins = rankInstincts(all, query, max)
	}
	if len(ins) == 0 {
		return
	}
	sys := buildInstinctSystem(ins)
	if sys == "" {
		return
	}
	// "augment" mode: sisip setelah blok system caller, jangan dominasi persona.
	req.Messages = injectSystem(req.Messages, sys, "augment")
	labels := make([]string, len(ins))
	for i, d := range ins {
		labels[i] = strings.TrimPrefix(d.Room, "instinct_")
	}
	log.Printf("flow_router instinct: injected %d %v (query=%.40q)", len(ins), labels, query)
}

// rankInstincts — PURE (no DB, testable): relevansi token-overlap × importance.
//
//	score = importance × (1 + 2·overlap)
//
// Anti-noise: overlap 0 + importance < fondasi → skip (jangan inject insting ga
// relevan = noise + boros). Insting fondasi (importance tinggi) tetap kandidat
// walau overlap 0 (refleks universal yg owner mau selalu ada).
func rankInstincts(all []brain.InstinctDrawer, query string, max int) []brain.InstinctDrawer {
	qTokens := tokenSet(query)
	type scored struct {
		d     brain.InstinctDrawer
		score float64
	}
	ranked := make([]scored, 0, len(all))
	for _, d := range all {
		overlap := overlapCount(qTokens, tokenSet(d.Content+" "+d.Room))
		if overlap == 0 && d.Importance < instinctFoundationImp {
			continue
		}
		imp := d.Importance
		if imp < 1 {
			imp = 1
		}
		ranked = append(ranked, scored{d, imp * float64(1+2*overlap)})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	out := make([]brain.InstinctDrawer, 0, max)
	for i := 0; i < len(ranked) && i < max; i++ {
		out = append(out, ranked[i].d)
	}
	return out
}

// buildInstinctSystem — render insting jadi system message "augment".
func buildInstinctSystem(ds []brain.InstinctDrawer) string {
	if len(ds) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Insting — refleks WHEN→THEN (kapan pakai kapabilitas yg lo punya)\n")
	b.WriteString("Insting relevan buat permintaan ini (PATUHI sbg refleks, bukan saran):\n\n")
	for i, d := range ds {
		domain := strings.TrimPrefix(d.Room, "instinct_")
		content := strings.TrimSpace(d.Content)
		if content == "" {
			continue
		}
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, domain, content)
	}
	return strings.TrimSpace(b.String())
}
