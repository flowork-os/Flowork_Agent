// codemap_tool.go — registrasi 5 tools untuk warga AI bisa mapping codebase
// langsung dari DB.
//
// Tools (implementasi terpisah):
//   codemap_search  — cari file di index by nama/path/package         (codemap_tool_search.go)
//   codemap_deps    — lihat deps + dependents satu file               (codemap_tool_deps.go)
//   codemap_impact  — blast radius: jika file X berubah, apa terdampak? (codemap_tool_impact.go)
//   codemap_zombies — list file orphan (dead code candidate)          (codemap_tool_zombies.go)
//   codemap_health  — health report file-file paling "sakit"          (codemap_tool_health.go)
//
// Semua tool baca langsung dari codemap_nodes + codemap_edges di brain SQLite.
// Tidak perlu HTTP — bisa dipanggil dari proses warga manapun.
package tools

// NewCodemapTools return semua 5 codemap tools — dipanggil dari DefaultRegistry/WatcherRegistry.
func NewCodemapTools(workspace string) []Tool {
	return []Tool{
		NewCodemapSearchTool(workspace),
		NewCodemapDepsTool(workspace),
		NewCodemapImpactTool(workspace),
		NewCodemapZombiesTool(workspace),
		NewCodemapHealthTool(workspace),
	}
}

// registerCodemapTools — register semua codemap tools ke registry.
func registerCodemapTools(registry *Registry, workspace string) {
	for _, tool := range NewCodemapTools(workspace) {
		registry.MustRegister(tool)
	}
}
