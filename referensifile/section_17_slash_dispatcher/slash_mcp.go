package tui

import "github.com/teetah2402/flowork/internal/tools"

func (m *model) handleMCPCommand(args []string) {
	if len(args) == 0 {
		m.appendLocal("MCP", "Usage: /mcp list | /mcp install <name>")
		return
	}

	switch args[0] {
	case "list":
		if m.mcpRegistry != nil {
			m.appendLocal("MCP", m.mcpRegistry.ConnectedServerInfo())
		} else {
			m.appendLocal("MCP", tools.ListOfficialMCPText())
		}
	case "install":
		if len(args) < 2 {
			m.appendLocal("MCP", "Usage: /mcp install <name>\n\n"+tools.ListOfficialMCPText())
			return
		}
		result, err := tools.InstallMCPServer(m.workspace, args[1])
		if err != nil {
			m.appendLocal("MCP", "Error: "+err.Error())
		} else {
			m.appendLocal("MCP", result)
		}
	default:
		m.appendLocal("MCP", "Unknown MCP command. Usage: /mcp list | /mcp install <name>")
	}
}
