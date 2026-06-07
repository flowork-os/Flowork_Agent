package tools

import "testing"

// TestProtectorEvasionNormalized — pre-freeze hardening: whitespace/path evasions
// of the immutable baseline must be caught by normalizeForMatch in protectorBlockHit.
func TestProtectorEvasionNormalized(t *testing.T) {
	blocked := []struct {
		name string
		args map[string]any
	}{
		{"plain rm -rf /", map[string]any{"command": "rm -rf /"}},
		{"multi-space", map[string]any{"command": "rm   -rf   /"}},
		{"tab-separated", map[string]any{"command": "rm\t-rf\t/"}},
		{"sudo tab", map[string]any{"cmd": "sudo\tcat x"}},
		{"path dot-segment", map[string]any{"path": "/etc/./shadow"}},
		{"path double-slash", map[string]any{"file": "/etc//shadow"}},
	}
	for _, c := range blocked {
		if hit, _ := protectorBlockHit(c.args); !hit {
			t.Errorf("%s: harusnya DIBLOK tapi lolos", c.name)
		}
	}
	// Benign must NOT trip.
	for _, c := range []map[string]any{
		{"command": "ls -la"},
		{"path": "/home/me/notes.txt"},
		{"command": "git status"},
	} {
		if hit, rule := protectorBlockHit(c); hit {
			t.Errorf("benign %v ke-blok salah (%s)", c, rule)
		}
	}
}
