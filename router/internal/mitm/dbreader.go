// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import (
	"database/sql"
	"strings"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/store"
)

var (
	aliasMu    sync.RWMutex
	aliasCache = map[string]string{}
)

func LoadAliasCache() error {
	d, err := store.Open()
	if err != nil {
		return err
	}
	rows, err := d.Query(`SELECT k, v FROM kv WHERE k LIKE ? ORDER BY k ASC`, "mitm:alias:%")
	if err != nil {
		return err
	}
	defer rows.Close()
	fresh := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		alias := strings.TrimPrefix(k, "mitm:alias:")
		fresh[alias] = v
	}
	aliasMu.Lock()
	aliasCache = fresh
	aliasMu.Unlock()
	return nil
}

func GetMitmAlias(tool, rawModel string) string {
	if rawModel == "" || tool == "" {
		return ""
	}
	if syn, ok := ModelSynonyms[tool]; ok {
		if alias, ok := syn[rawModel]; ok {
			return alias
		}
	}
	for _, p := range ModelPatterns[tool] {
		if p.Match.MatchString(rawModel) {
			return p.Alias
		}
	}
	return ""
}

func LookupAlias(key string) string {
	aliasMu.RLock()
	defer aliasMu.RUnlock()
	return aliasCache[key]
}

func InjectAliasForTest(d *sql.DB, key, value string) error {
	_, err := d.Exec(
		`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(k) DO UPDATE SET v = excluded.v, updatedAt = excluded.updatedAt`,
		"mitm:alias:"+key, value)
	return err
}
