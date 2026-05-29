package tools

// warga_registry.go — per-warga filtered registry berdasarkan capability matrix
// (rc174 hybrid role-default + per-warga override).
//
// WargaRegistry = DefaultRegistry filtered via wargacaps.LoadFor(workspace, wargaName).
// Pasca rc174, daemon yang spawn warga harus pakai WargaRegistry instead of
// DefaultRegistry/WatcherRegistry/pelayanRegistry untuk respect Ayah's GUI toggles.

import (
	"fmt"
	"os"

	"github.com/teetah2402/flowork/internal/wargacaps"
)

// WargaRegistry build full registry, lalu filter via capability matrix.
//
// Lookup precedence (per warga + tool):
//  1. warga_capability_overrides (per-individual)
//  2. role_capabilities (per-role default)
//  3. NO entry → tool excluded (deny by default)
//
// Empty wargaName → fall back ke DefaultRegistry (full power, back-compat
// untuk caller yang belum migrate).
func WargaRegistry(workspace, wargaName string) *Registry {
	full := DefaultRegistry(workspace)
	if wargaName == "" {
		return full
	}
	caps, err := wargacaps.LoadFor(workspace, wargaName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wargacaps] LoadFor(%s) error: %v — fallback to full registry\n", wargaName, err)
		return full
	}
	if len(caps) == 0 {
		// No caps loaded — could be: (a) warga ga di registry, (b) seed belum
		// jalan. Fallback to full registry dengan log warning supaya Ayah tau.
		fmt.Fprintf(os.Stderr, "[wargacaps] no caps for warga=%q — fallback to full registry (run SeedRoleDefaults?)\n", wargaName)
		return full
	}
	return full.Filter(func(name string) bool {
		return caps.Has(name)
	})
}
