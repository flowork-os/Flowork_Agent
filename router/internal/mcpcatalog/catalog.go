// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mcpcatalog

type Plugin struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Transport   string   `json:"transport"`
	URL         string   `json:"url,omitempty"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	OAuth       bool     `json:"oauth,omitempty"`
	Extension   string   `json:"extensionUrl,omitempty"`
	ToolNames   []string `json:"toolNames,omitempty"`
}

var DefaultPlugins = []Plugin{
	{
		Name:        "exa",
		Title:       "Exa",
		Description: "Real-time web search + code documentation lookup via Exa's hosted MCP.",
		Transport:   "http",
		URL:         "https://mcp.exa.ai/mcp",
		OAuth:       false,
		ToolNames:   []string{"web_search_exa", "web_fetch_exa"},
	},
	{
		Name:        "tavily",
		Title:       "Tavily",
		Description: "Web search optimised for LLM agents (search + extract + crawl + map).",
		Transport:   "http",
		URL:         "https://mcp.tavily.com/mcp",
		OAuth:       true,
		ToolNames:   []string{"tavily_search", "tavily_extract", "tavily_crawl", "tavily_map"},
	},
	{
		Name:        "browsermcp",
		Title:       "Browser MCP",
		Description: "Drive your running Chrome instance (requires the Browser MCP extension).",
		Transport:   "stdio",
		Command:     "npx",
		Args:        []string{"-y", "@browsermcp/mcp@latest"},
		Extension:   "https://chromewebstore.google.com/detail/browser-mcp-automate-your/bjfgambnhccakkhmkepdoekmckoijdlc",
		ToolNames: []string{
			"browser_navigate", "browser_snapshot", "browser_click", "browser_type",
			"browser_screenshot", "browser_get_console_logs", "browser_wait",
			"browser_press_key", "browser_go_back", "browser_go_forward",
		},
	},
}

var custom []Plugin

func Catalog() []Plugin {
	out := make([]Plugin, 0, len(DefaultPlugins)+len(custom))
	seen := map[string]bool{}
	for _, p := range DefaultPlugins {
		if p.Name == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
	}
	for _, p := range custom {
		if p.Name == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		out = append(out, p)
	}
	return out
}

func Register(p Plugin) {
	if p.Name == "" {
		return
	}

	filtered := custom[:0]
	for _, e := range custom {
		if e.Name != p.Name {
			filtered = append(filtered, e)
		}
	}
	custom = append(filtered, p)
}

func Set(list []Plugin) {
	custom = append(custom[:0], list...)
}

func Lookup(name string) (Plugin, bool) {
	for _, p := range Catalog() {
		if p.Name == name {
			return p, true
		}
	}
	return Plugin{}, false
}
