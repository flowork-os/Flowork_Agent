package brain

import (
	"strings"
	"testing"
)

// TestCodemapTablesFormat — canonical lower_snake_case naming.
func TestCodemapTablesFormat(t *testing.T) {
	for _, tbl := range AllCodemapTables() {
		if tbl == "" {
			t.Errorf("empty table name in registry")
			continue
		}
		if strings.ToLower(tbl) != tbl {
			t.Errorf("table %q must be lowercase", tbl)
		}
		if !strings.HasPrefix(tbl, "codemap_") {
			t.Errorf("table %q must have codemap_ prefix", tbl)
		}
	}
}

// TestCodemapColumnsUnique — no duplicate among node + edge columns.
func TestCodemapColumnsUnique(t *testing.T) {
	seen := map[string]int{}
	for _, c := range AllCodemapNodeColumns() {
		seen[c]++
	}
	for _, c := range AllCodemapEdgeColumns() {
		seen[c]++
	}
	// Allow path/from_path duplicate kalau ada — tapi current registry no overlap.
	for k, count := range seen {
		if count > 1 {
			t.Errorf("column %q registered %d times across registries", k, count)
		}
	}
}

// TestCodemapColumnsNonEmpty — assert all columns ngga empty string.
func TestCodemapColumnsNonEmpty(t *testing.T) {
	all := append([]string{}, AllCodemapNodeColumns()...)
	all = append(all, AllCodemapEdgeColumns()...)
	for _, c := range all {
		if c == "" {
			t.Error("empty column name in registry")
		}
		if strings.ContainsAny(c, " \t\n") {
			t.Errorf("column %q contains whitespace", c)
		}
	}
}
