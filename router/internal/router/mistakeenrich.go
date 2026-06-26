// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

const (
	antibodyMaxInject = 3

	antibodyUniversalKarma = 10

	antibodyMistakeTier = "global"

	antibodyHalfLifeDays = 30.0

	antibodyRecencyFloor = 0.1
)

func maybeInjectAntibodies(ctx context.Context, req *OpenAIRequest, settings *store.Settings) {
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
	ab := relevantAntibodies(ctx, query, antibodyMaxInject)
	if len(ab) == 0 {
		return
	}
	sys := buildAntibodySystem(ab)
	if sys == "" {
		return
	}

	req.Messages = injectSystem(req.Messages, sys, "augment")
	log.Printf("flow_router antibody: injected %d antibody (query=%.48q)", len(ab), query)
}

func relevantAntibodies(ctx context.Context, query string, max int) []brain.Mistake {
	all, err := brain.ListMistakes(ctx, antibodyMistakeTier, "", 200)
	if err != nil || len(all) == 0 {
		return nil
	}
	return rankAntibodies(all, query, max, time.Now())
}

func rankAntibodies(all []brain.Mistake, query string, max int, now time.Time) []brain.Mistake {
	qTokens := tokenSet(query)
	type scored struct {
		m     brain.Mistake
		score float64
	}
	ranked := make([]scored, 0, len(all))
	for _, m := range all {
		overlap := overlapCount(qTokens, tokenSet(m.Title+" "+m.Content+" "+m.Category))
		if overlap == 0 && m.HitCount < antibodyUniversalKarma {
			continue
		}
		karma := m.HitCount
		if karma < 1 {
			karma = 1
		}
		rec := recencyFactor(m.UpdatedAt, now)
		ranked = append(ranked, scored{m, float64(karma) * float64(1+2*overlap) * rec})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	out := make([]brain.Mistake, 0, max)
	for i := 0; i < len(ranked) && i < max; i++ {
		out = append(out, ranked[i].m)
	}
	return out
}

func recencyFactor(updatedAt string, now time.Time) float64 {
	t, ok := parseMistakeTime(updatedAt)
	if !ok {
		return antibodyRecencyFloor
	}
	days := now.Sub(t).Hours() / 24
	if days < 0 {
		days = 0
	}
	f := math.Pow(0.5, days/antibodyHalfLifeDays)
	if f < antibodyRecencyFloor {
		f = antibodyRecencyFloor
	}
	if f > 1 {
		f = 1
	}
	return f
}

func parseMistakeTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func buildAntibodySystem(ms []brain.Mistake) string {
	if len(ms) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Antibodi — kesalahan TERBUKTI, JANGAN diulang\n")
	b.WriteString("Pelajaran dari kesalahan lampau (\"kuat\" = berapa kali kecatat; makin tinggi makin wajib dipatuhi):\n\n")
	for i, m := range ms {
		title := strings.TrimSpace(m.Title)
		content := strings.TrimSpace(m.Content)
		fmt.Fprintf(&b, "%d. [%s · kuat %d] %s", i+1, m.Category, m.HitCount, title)
		if content != "" {
			fmt.Fprintf(&b, " — %s", content)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(f) < 3 {
			continue
		}
		if _, stop := antibodyStopwords[f]; stop {
			continue
		}
		out[f] = struct{}{}
	}
	return out
}

func overlapCount(a, b map[string]struct{}) int {

	if len(b) < len(a) {
		a, b = b, a
	}
	n := 0
	for t := range a {
		if _, ok := b[t]; ok {
			n++
		}
	}
	return n
}

var antibodyStopwords = map[string]struct{}{
	"yang": {}, "untuk": {}, "dari": {}, "dengan": {}, "dan": {}, "atau": {},
	"the": {}, "and": {}, "for": {}, "with": {}, "this": {}, "that": {},
	"buat": {}, "kalau": {}, "saja": {}, "lagi": {}, "sudah": {}, "akan": {},
}
