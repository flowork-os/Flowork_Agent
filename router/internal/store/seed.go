// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"database/sql"
	"encoding/json"
	"strings"
)

func parseSyncBundle(raw []byte) (*SyncBundle, error) {
	var b SyncBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

const SeedKeyPlaceholder = "PASTE_YOUR_KEY_HERE"

func ExportSeed(d *sql.DB) *SyncBundle {
	b := ExportConfig(d)

	for i := range b.Providers {

		b.Providers[i].Email = ""
		data := b.Providers[i].Data
		if data == nil {
			continue
		}

		if v, ok := data[CfgAPIKey].(string); ok && v != "" {
			if b.Providers[i].AuthType == AuthTypeAPIKey {
				data[CfgAPIKey] = SeedKeyPlaceholder
			} else {
				data[CfgAPIKey] = ""
			}
		}

		if hdr, ok := data[CfgHeaders].(map[string]any); ok {
			for k := range hdr {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "authorization") || strings.Contains(lk, "api-key") ||
					strings.Contains(lk, "cookie") || strings.Contains(lk, "token") {
					hdr[k] = ""
				}
			}
		}
	}

	for i := range b.MediaProviders {
		if b.MediaProviders[i].APIKey != "" {
			b.MediaProviders[i].APIKey = SeedKeyPlaceholder
		}
	}

	for i := range b.MCPServers {
		for k := range b.MCPServers[i].Env {
			b.MCPServers[i].Env[k] = SeedKeyPlaceholder
		}
	}

	b.ProxyPools = nil

	return b
}

func SeedFromBundleJSON(d *sql.DB, raw []byte) map[string]int {
	if len(raw) == 0 {
		return nil
	}
	existing, _ := ListProviders(d)
	if len(existing) > 0 {
		return nil
	}
	b, err := parseSyncBundle(raw)
	if err != nil || b == nil || len(b.Providers) == 0 {
		return nil
	}
	return ImportConfig(d, b)
}
