// Package tools — get_protected_core_list: plug-and-play readonly view
// ke Protected Core rules yg saat ini aktif.
//
// Kenapa ada: doktrin warga (terutama aksara coder) bilang "HARAM sentuh
// Sistem Imun → Protector". Supaya doktrin itu portable (Ayah bisa tambah/
// kurang rules lewat GUI kapan aja tanpa edit prompt), warga butuh cara
// query list terkini runtime.
//
// Output mencakup:
//   - hardcoded_basenames: file names yg always-protected (compile-time list)
//   - hardcoded_suffixes: path suffix yg always-protected
//   - disabled_hardcoded: hardcoded entries yg di-disable Ayah via GUI
//   - dynamic_rules: user-defined rules dari state/protector/rules.json
//
// Warga HARUS panggil tool ini sebelum lanjut task yg sentuh file yg
// potentially protected, BUKAN hardcode mental-map pake list yg di doktrin.
package tools

import (
	"context"
	"encoding/json"
	"os"
	"sort"

	"github.com/teetah2402/flowork/internal/provider"
)

type GetProtectedCoreListTool struct {
	workspace string
}

func NewGetProtectedCoreListTool(workspace string) *GetProtectedCoreListTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &GetProtectedCoreListTool{workspace: workspace}
}

func (t *GetProtectedCoreListTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "get_protected_core_list",
		Description: "Readonly snapshot Protected Core rules yg aktif sekarang" +
			" (hardcoded basenames + suffixes + dynamic rules dari GUI Sistem" +
			" Imun → Protector + disabled hardcoded). Plug-and-play: list" +
			" dinamis, refresh otomatis kalau Ayah edit Protector. Pake" +
			" SEBELUM sentuh file yg potentially protected — JANGAN hardcode" +
			" list dari doktrin. Kalau path lo masuk salah satu kategori di" +
			" sini, HARAM sentuh.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *GetProtectedCoreListTool) Execute(_ context.Context, _ Invocation) (Result, error) {
	// Force a fresh reload so the response reflects the latest GUI edits.
	ensureDynamicFresh()

	basenamesMap := HardcodedBasenames()
	basenames := make([]string, 0, len(basenamesMap))
	for k := range basenamesMap {
		basenames = append(basenames, k)
	}
	sort.Strings(basenames)

	suffixes := HardcodedSuffixes()
	sort.Strings(suffixes)

	state := GetDynamicState()

	payload := map[string]any{
		"hardcoded_basenames": basenames,
		"hardcoded_suffixes":  suffixes,
		"disabled_hardcoded":  state.DisabledHardcoded,
		"dynamic_rules":       state.Rules,
		"rules_version":       state.Version,
		"note": "Hardcoded = compile-time (butuh code change). Dynamic =" +
			" GUI Protector, bisa edit live. Disabled = hardcoded yg Ayah" +
			" pilih off. WARGA: semua path di list ini HARAM disentuh lewat" +
			" tool write/edit/bash destructive. Kalau perlu modify: ticket_create" +
			" topic=protector-rule-request, escalate ke Ayah + @gerbang.",
	}
	out, _ := json.MarshalIndent(payload, "", "  ")
	return Result{ToolName: "get_protected_core_list", OK: true, Output: string(out)}, nil
}
