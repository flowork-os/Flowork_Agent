// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// CLI tool plug-and-play (RegisterCLITool) → dok lock/plug-and-play.md  ⚠️ FROZEN — jangan edit.
// Nambah CLI tool: file sibling cli_<x>.go + init(){ RegisterCLITool(...) }. Cara: CARAFREEZE.MD
// (POLA A). Pola freeze: lock/frozen-core.md

package clitools

var extraCLITools []Tool

func RegisterCLITool(t Tool) {
	if t.ID == "" {
		return
	}
	extraCLITools = append(extraCLITools, t)
}
