// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package router

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/store"
)

var canonicalTaskCategories = map[string]struct{}{
	"saham": {}, "crypto": {}, "music": {}, "promo": {},
}

const (
	antibodyFbCategory = "logic"
	antibodyFbTitle    = "task_run wajib kategori kanonik"
	antibodyFbContent  = "Saat user minta analisa, 'category' di task_run WAJIB dari daftar valid: saham, crypto, music, promo. JANGAN ngarang 'analysis'/'stock'/'security_stock'. 'subject' = entitas murni tanpa suffix '.JK'."
	antibodyFbHit      = 3
)

func maybeReinforceAntibody(ctx context.Context, resp *OpenAIResponse, settings *store.Settings) {
	if settings == nil || !settings.Brain.Enabled || resp == nil {
		return
	}
	if !brain.Available() {
		return
	}
	bad := detectNonCanonicalTaskRun(resp)
	if bad == "" {
		return
	}
	if _, _, err := brain.SubmitMistake(ctx, antibodyFbCategory, antibodyFbTitle,
		antibodyFbContent, "router-feedback", antibodyFbHit); err != nil {
		log.Printf("flow_router antibody-feedback: submit gagal: %v", err)
		return
	}
	log.Printf("flow_router antibody-feedback: halu kategori %q kedeteksi → karma antibody +%d", bad, antibodyFbHit)
}

func detectNonCanonicalTaskRun(resp *OpenAIResponse) string {
	if resp == nil {
		return ""
	}
	for _, ch := range resp.Choices {
		if len(ch.Message.ToolCalls) == 0 {
			continue
		}
		var calls []openAIToolCall
		if json.Unmarshal(ch.Message.ToolCalls, &calls) != nil {
			continue
		}
		for _, c := range calls {
			if c.Function.Name != "task_run" || c.Function.Arguments == "" {
				continue
			}
			var args struct {
				Category string `json:"category"`
			}
			if json.Unmarshal([]byte(c.Function.Arguments), &args) != nil {
				continue
			}
			cat := strings.ToLower(strings.TrimSpace(args.Category))
			if cat == "" {
				continue
			}
			if _, ok := canonicalTaskCategories[cat]; !ok {
				return cat
			}
		}
	}
	return ""
}
