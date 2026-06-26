// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package menus

import (
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/api"
	"github.com/flowork-os/flowork_Router/cmd/flow-cli/utils"
)

func CLITools(c *api.Client) error {
	return utils.RunMenu("CLI Tools", []utils.MenuItem{
		{Label: "All-tool statuses", Action: func() error { return allStatuses(c) }},
		{Label: "Show Claude Code config", Action: func() error { return showTool(c, "claude-settings") }},
		{Label: "Show Codex config", Action: func() error { return showTool(c, "codex-settings") }},
		{Label: "Show Cline config", Action: func() error { return showTool(c, "cline-settings") }},
		{Label: "Show Hermes config", Action: func() error { return showTool(c, "hermes-settings") }},
		{Label: "Show OpenCode config", Action: func() error { return showTool(c, "opencode-settings") }},
		{Label: "Show Kilo config", Action: func() error { return showTool(c, "kilo-settings") }},
		{Label: "Show Droid config", Action: func() error { return showTool(c, "droid-settings") }},
		{Label: "Show Cowork MCP registry", Action: func() error { return showTool(c, "cowork-mcp-registry") }},
		{Label: "Show Cowork MCP tools", Action: func() error { return showTool(c, "cowork-mcp-tools") }},
	})
}

func allStatuses(c *api.Client) error {
	var out map[string]any
	if err := c.Get("/api/cli-tools/all-statuses", &out); err != nil {
		return err
	}
	utils.Info(utils.PrettyJSON(out))
	return nil
}

func showTool(c *api.Client, subpath string) error {
	var out map[string]any
	if err := c.Get("/api/cli-tools/"+subpath, &out); err != nil {
		return err
	}
	utils.Info(utils.PrettyJSON(out))
	return nil
}
