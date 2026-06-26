package clitools

import "testing"

// TestRegisterCLITool buktiin CLI tool BARU via RegisterCLITool muncul di All() — tanpa edit
// hardcode built-in. Built-in (claude) tetap ada.
func TestRegisterCLITool(t *testing.T) {
	baseHasClaude := false
	for _, tl := range All() {
		if tl.ID == "claude" {
			baseHasClaude = true
		}
	}
	if !baseHasClaude {
		t.Fatal("built-in claude harus tetap ada")
	}
	RegisterCLITool(Tool{ID: "dummy-cli", DisplayName: "Dummy", BinaryName: "dummy"})
	found := false
	for _, tl := range All() {
		if tl.ID == "dummy-cli" {
			found = true
		}
	}
	if !found {
		t.Fatal("CLI tool baru via RegisterCLITool harus muncul di All()")
	}
}
