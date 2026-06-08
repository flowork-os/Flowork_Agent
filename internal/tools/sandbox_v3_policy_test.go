package tools

import (
	"strings"
	"testing"
)

// TestRequiresApproval covers the plug-and-play approval chokepoint: read-only calls
// are exempt, sensitive args gate even reads, non-sensitive mutations stay un-gated
// (legacy), and a registered ExtraGatePolicy can add gating.
func TestRequiresApproval(t *testing.T) {
	defer func() { ReadOnlyClassifier = nil; ExtraGatePolicy = nil }()

	ReadOnlyClassifier = func(name string, args map[string]any) bool {
		if name == "reader" {
			return true
		}
		if c, ok := args["command"].(string); ok {
			return strings.HasPrefix(c, "ls") || strings.HasPrefix(c, "cat ")
		}
		return false
	}

	// read-only tool → exempt
	if requiresApproval("reader", nil) {
		t.Error("read-only tool must be exempt from approval")
	}
	// read-only command → exempt
	if requiresApproval("shell", map[string]any{"command": "ls -la"}) {
		t.Error("read-only command must be exempt")
	}
	// sensitive args (state.db) → gated EVEN for a read (safety: not loosened)
	if !requiresApproval("shell", map[string]any{"command": "cat state.db"}) {
		t.Error("sensitive args must gate even a read-only command")
	}
	// non-sensitive mutation → not gated by default (legacy behaviour preserved)
	if requiresApproval("shell", map[string]any{"command": "rm y"}) {
		t.Error("non-sensitive mutation must not be gated by default")
	}
	// extra policy can add a gate without touching the locked file
	ExtraGatePolicy = func(n string, _ map[string]any) bool { return n == "danger" }
	if !requiresApproval("danger", nil) {
		t.Error("ExtraGatePolicy must be able to require approval")
	}
	if requiresApproval("safe", nil) {
		t.Error("ExtraGatePolicy must not gate unrelated tools")
	}
}
